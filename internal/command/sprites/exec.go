package sprites

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	sprites "github.com/superfly/sprites-go"
	"golang.org/x/sync/errgroup"

	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/iostreams"
)

// detachChar is Ctrl+\. Typing it in a TTY session disconnects and leaves the
// session running on the sprite.
const detachChar = 0x1C

const (
	// interruptedExitCode is what a shell reports for a command ended by
	// Ctrl+C.
	interruptedExitCode = 130

	// interruptGrace is how long an interrupted remote command gets to exit
	// before the connection is dropped anyway.
	interruptGrace = 5 * time.Second
)

const (
	// A command that cannot connect is retried this many times in total,
	// waiting startRetryDelay and then twice as long each time.
	defaultStartAttempts = 5
	startRetryMaxDelay   = 16 * time.Second

	// startAttemptsEnv overrides defaultStartAttempts. The sprite CLI reads
	// the same variable.
	startAttemptsEnv = "SPRITE_EXEC_MAX_RETRIES"
)

// startRetryDelay is the wait before the first retry. It is a variable so
// tests need not wait for it.
var startRetryDelay = time.Second

func startAttempts() int {
	if n, err := strconv.Atoi(os.Getenv(startAttemptsEnv)); err == nil && n > 0 {
		return n
	}

	return defaultStartAttempts
}

// httpStatusRegex finds the status code in the SDK's plain-text errors.
var httpStatusRegex = regexp.MustCompile(`(?i)\b(?:status|HTTP)[ :]+(\d{3})\b`)

// isTransient reports whether a failure to start a command is safe to retry.
// That means the request provably never reached the sprite: the connection
// could not be made, or a proxy answered that nothing was there to take it.
// A timeout or a reset connection is not retried, because the command may
// already be running and running it again could do its work twice.
func isTransient(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	unavailable := func(status int) bool {
		return status == http.StatusBadGateway || status == http.StatusServiceUnavailable
	}

	var apiErr *sprites.APIError
	if errors.As(err, &apiErr) {
		return unavailable(apiErr.StatusCode)
	}

	msg := err.Error()

	if m := httpStatusRegex.FindStringSubmatch(msg); m != nil {
		status, _ := strconv.Atoi(m[1])
		return unavailable(status)
	}

	msg = strings.ToLower(msg)

	for _, sign := range []string{"connection refused", "no such host", "temporary failure in name resolution"} {
		if strings.Contains(msg, sign) {
			return true
		}
	}

	return false
}

// terminalEnv is the local environment forwarded to TTY sessions so the remote
// terminal behaves like the local one.
var terminalEnv = []string{"TERM", "COLORTERM", "LANG", "LC_ALL"}

func newExec() *cobra.Command {
	const (
		short = "Run a command in a sprite."

		long = `Run a command in a sprite and exit with its exit status.

Everything after the command name is passed to the command, so flags for
'fly sprites exec' itself must come before it. Use -- to make the split
explicit.

Without --tty the command's output is streamed and stdin is forwarded only
when it is piped or --stdin is passed. With --tty a pseudo-terminal is
allocated; press Ctrl+\ to detach and leave the command running, then use
'fly sprites attach' to reconnect.

Ports the command opens are forwarded to the same port on localhost while it
runs, unless --no-port-forward is passed.
`
	)

	cmd := newCommand("exec [flags] -- <command> [args...]", short, long, runExec)
	cmd.Aliases = []string{"x"}
	cmd.Args = cobra.ArbitraryArgs
	cmd.Example = `  fly sprites exec -- ls -la
  fly sprites exec --dir /app --env KEY=value,FOO=bar -- env
  printf 'hello' | fly sprites exec --stdin -- cat
  fly sprites exec --tty -- /bin/bash
  fly sprites exec -o my-org -s my-sprite -- npm start`

	// Stop parsing flags at the command name so its own flags reach it.
	cmd.Flags().SetInterspersed(false)

	flag.Add(cmd,
		flag.String{
			Name:        "dir",
			Description: "Working directory for the command",
		},
		flag.Bool{
			Name:        "tty",
			Description: "Allocate a pseudo-terminal",
		},
		flag.StringArray{
			Name:        "env",
			Description: "Environment variables as KEY=value. Repeat the flag, or give it once with comma-separated pairs",
		},
		flag.Bool{
			Name:        "stdin",
			Description: "Read and forward stdin (without --tty)",
		},
		flag.Bool{
			Name:        "no-stdin",
			Description: "Do not read or forward stdin (without --tty)",
		},
		flag.Bool{
			Name:        "no-port-forward",
			Description: "Do not forward ports opened by the command to localhost",
		},
		flag.StringArray{
			Name:        "file",
			Description: "Upload a local file before running, as source:dest (can be repeated)",
		},
	)

	return cmd
}

func runExec(ctx context.Context, s *session) error {
	io := iostreams.FromContext(ctx)

	args := flag.Args(ctx)
	if len(args) == 0 {
		return flyerr.WithSuggestion(
			errors.New("a command is required"),
			"Use 'fly sprites console' for an interactive shell, or 'fly sprites sessions' to manage running sessions.",
		)
	}

	var (
		tty     = flag.GetBool(ctx, "tty")
		stdin   = flag.GetBool(ctx, "stdin")
		noStdin = flag.GetBool(ctx, "no-stdin")
		dir     = flag.GetString(ctx, "dir")
	)

	switch {
	case stdin && noStdin:
		return errors.New("--stdin and --no-stdin cannot be used together")
	case stdin && tty:
		return errors.New("--stdin is not supported with --tty")
	case noStdin && tty:
		return errors.New("--no-stdin is not supported with --tty")
	}

	files, err := parseFileSpecs(flag.GetStringArray(ctx, "file"))
	if err != nil {
		return err
	}

	sprite, err := s.sprite(ctx, "")
	if err != nil {
		return err
	}

	if err := uploadFiles(ctx, io, sprite, dir, files); err != nil {
		return err
	}

	opts := remoteOptions{
		args:        args,
		dir:         dir,
		env:         parseEnv(flag.GetStringArray(ctx, "env")),
		tty:         tty,
		portForward: !flag.GetBool(ctx, "no-port-forward"),
	}

	// Forward stdin when asked to, or when something is piped in.
	if !tty && !noStdin && (stdin || !io.IsStdinTTY()) {
		opts.stdin = io.In
	}

	return runRemote(ctx, sprite, opts)
}

// envPairStart matches the start of a KEY= pair that follows a comma.
var envPairStart = regexp.MustCompile(`,\s*[A-Za-z_][A-Za-z0-9_]*=`)

// parseEnv turns --env values into KEY=value pairs. Given once, the value may
// hold several comma-separated pairs, as the sprite CLI accepts. Given more
// than once, each value is one pair taken literally, which is the way to pass
// a value that itself contains ",NAME=" text, such as a connection string.
func parseEnv(values []string) []string {
	if len(values) == 1 {
		return splitEnv(values[0])
	}

	var env []string

	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			env = append(env, value)
		}
	}

	return env
}

// splitEnv splits comma-separated KEY=value pairs. A comma only separates
// pairs when another KEY= follows it, so a value may contain plain commas.
func splitEnv(value string) []string {
	var env []string

	for {
		loc := envPairStart.FindStringIndex(value)
		if loc == nil {
			break
		}

		if pair := strings.TrimSpace(value[:loc[0]]); pair != "" {
			env = append(env, pair)
		}

		value = value[loc[0]+1:]
	}

	if pair := strings.TrimSpace(value); pair != "" {
		env = append(env, pair)
	}

	return env
}

type fileSpec struct {
	source string
	dest   string
}

func parseFileSpecs(values []string) ([]fileSpec, error) {
	specs := make([]fileSpec, 0, len(values))

	for _, value := range values {
		// Skip the colon of a Windows drive letter, as in C:\dir\file:/dest.
		skip := 0
		if runtime.GOOS == "windows" && len(value) > 2 && value[1] == ':' &&
			(value[2] == '\\' || value[2] == '/') && isLetter(value[0]) {
			skip = 2
		}

		source, dest, ok := strings.Cut(value[skip:], ":")
		source = value[:skip] + source

		if !ok || source == "" || dest == "" {
			return nil, fmt.Errorf("invalid --file %q: expected source:dest", value)
		}

		specs = append(specs, fileSpec{source: source, dest: dest})
	}

	return specs, nil
}

func isLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// uploadFiles writes local files into the sprite before a command runs.
// Relative destinations are resolved against dir, or the sprite user's home
// directory when it is empty.
func uploadFiles(ctx context.Context, io *iostreams.IOStreams, sprite *sprites.Sprite, dir string, files []fileSpec) error {
	if len(files) == 0 {
		return nil
	}

	// Without --dir, resolve relative destinations where `file push` does.
	if dir == "" {
		dir = remoteHome
	}

	fsys := sprite.FilesystemAt(dir)

	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)

	for _, file := range files {
		g.Go(func() error {
			info, err := os.Stat(file.source)
			if err != nil {
				return fmt.Errorf("failed to read %s: %w", file.source, err)
			}

			data, err := os.ReadFile(file.source)
			if err != nil {
				return fmt.Errorf("failed to read %s: %w", file.source, err)
			}

			// Keep the local permissions, so an uploaded script stays
			// executable.
			if err := fsys.WriteFileContext(gctx, file.dest, data, info.Mode().Perm()); err != nil {
				return fmt.Errorf("failed to upload %s to %s: %w", file.source, file.dest, err)
			}

			mu.Lock()
			defer mu.Unlock()

			fmt.Fprintf(io.ErrOut, "Uploaded %s -> %s (%d bytes)\n", file.source, file.dest, len(data))

			return nil
		})
	}

	return g.Wait()
}

// remoteOptions describes a command to run in a sprite, or a session to
// attach to.
type remoteOptions struct {
	// args is the command to run. It is unused when attaching.
	args []string
	dir  string
	env  []string
	tty  bool

	// stdin is forwarded to a non-TTY command. TTY commands always read the
	// terminal.
	stdin io.Reader

	portForward bool

	// sessionID attaches to an existing session instead of starting a
	// command.
	sessionID string

	// replay asks a non-TTY session for its output from the beginning.
	replay bool

	// detachMessage is printed when the user detaches with Ctrl+\.
	detachMessage string
}

// runRemote runs a command in a sprite, wiring it to the local terminal, and
// returns an ExitCodeError carrying its exit status when that is not zero.
func runRemote(ctx context.Context, sprite *sprites.Sprite, opts remoteOptions) error {
	io := iostreams.FromContext(ctx)

	// The command gets a context of its own, so that an interrupt can be
	// relayed to the remote process before the connection is dropped.
	parent := ctx

	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	defer cancel()

	// The terminal the user is typing at, when there is one and the command
	// wants a pty.
	var tty localTerminal
	if opts.tty {
		tty = openTerminal()
	}

	interactive := tty != nil

	detached := make(chan struct{})

	env := opts.env
	if opts.tty {
		env = withTerminalEnv(env)
	}

	// build makes a fresh command, since one that failed to start cannot be
	// started again.
	build := func() (*sprites.Cmd, func()) {
		var cmd *sprites.Cmd
		if opts.sessionID != "" {
			cmd = sprite.AttachSessionContext(ctx, opts.sessionID)
		} else {
			cmd = sprite.CommandContext(ctx, opts.args[0], opts.args[1:]...)
			cmd.Dir = opts.dir
		}

		if len(env) > 0 {
			cmd.Env = env
		}

		cmd.Stdout = io.Out
		cmd.Stderr = io.ErrOut

		switch {
		case opts.tty:
			cmd.SetTTY(true)

			// Ctrl+\ only means "detach" when it is typed. Piped input may
			// hold that byte as data.
			if interactive {
				if rows, cols, ok := tty.Size(); ok {
					_ = cmd.SetTTYSize(rows, cols)
				}

				cmd.Stdin = &detachReader{r: tty.Input(), detached: detached}
			} else {
				cmd.Stdin = os.Stdin
			}
		case opts.sessionID != "":
			if opts.replay {
				cmd.SetOutputOffset(0)
			}

			cmd.Stdin = opts.stdin
		default:
			cmd.Stdin = opts.stdin
		}

		return cmd, forwardPorts(ctx, cmd, sprite, opts.portForward)
	}

	// Connecting can fail briefly, for instance while a paused sprite wakes.
	// Only failures where the command cannot have started are retried.
	var (
		cmd            *sprites.Cmd
		stopForwarding func()
	)

	for attempt, delay := 1, startRetryDelay; ; attempt, delay = attempt+1, min(delay*2, startRetryMaxDelay) {
		cmd, stopForwarding = build()

		startDone := make(chan error, 1)
		go func() { startDone <- cmd.Start() }()

		var err error

		select {
		case err = <-startDone:
		case <-parent.Done():
			// Interrupted while connecting. The request may already have
			// reached the sprite, so see the start through: if the command
			// did start, the interrupt is relayed to it below instead of
			// leaving it running unattended.
			select {
			case err = <-startDone:
			case <-time.After(interruptGrace):
				err = context.Canceled
			}

			if err != nil {
				stopForwarding()
				return &flyerr.ExitCodeError{Code: interruptedExitCode}
			}
		}

		if err == nil {
			break
		}

		stopForwarding()

		if attempt >= startAttempts() || !isTransient(err) {
			return fmt.Errorf("failed to start command: %w", err)
		}

		fmt.Fprintf(io.ErrOut, "Connection failed (%v), retrying in %s...\n", err, delay)

		select {
		case <-parent.Done():
			return &flyerr.ExitCodeError{Code: interruptedExitCode}
		case <-time.After(delay):
		}
	}
	defer stopForwarding()

	restore := func() {}

	if interactive {
		restoreMode, err := tty.MakeRaw()
		if err != nil {
			return fmt.Errorf("failed to set the terminal to raw mode: %w", err)
		}

		var once sync.Once
		restore = func() {
			once.Do(func() {
				// Only a terminal understands these; a redirected stdout
				// would get them as stray bytes.
				if io.IsStdoutTTY() {
					resetTerminal(io.Out)
				}

				restoreMode()
			})
		}
		defer restore()

		// The size given before the start is not applied to the new terminal,
		// so send it again now that the command is running.
		if rows, cols, ok := tty.Size(); ok {
			_ = cmd.SetTTYSize(rows, cols)
		}

		stopResize := tty.WatchResize(func(rows, cols uint16) {
			_ = cmd.SetTTYSize(rows, cols)
		})
		defer stopResize()
	}

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	select {
	case <-detached:
		// Dropping the connection leaves the session running on the sprite.
		cancel()
		restore()

		msg := opts.detachMessage
		if msg == "" {
			msg = "Detached. Reconnect with 'fly sprites attach'."
		}
		fmt.Fprintln(io.ErrOut, msg)

		return nil
	case <-parent.Done():
		// Interrupted locally. For a command this run started, dropping the
		// connection would leave it running, so ask it to stop and give it a
		// moment to. A session this run only attached to is someone else's to
		// stop: Ctrl+C there just stops watching.
		if opts.sessionID == "" {
			if err := cmd.Signal("INT"); err == nil {
				select {
				case <-waitDone:
				case <-time.After(interruptGrace):
				}
			}
		}

		cancel()
		restore()

		return &flyerr.ExitCodeError{Code: interruptedExitCode}
	case err := <-waitDone:
		restore()

		var exitErr *sprites.ExitError

		switch {
		case err == nil:
			return nil
		case errors.As(err, &exitErr):
			return &flyerr.ExitCodeError{Code: exitErr.ExitCode()}
		default:
			return err
		}
	}
}

// withTerminalEnv adds the local terminal's environment to env, without
// overriding anything already set there.
func withTerminalEnv(env []string) []string {
	set := make(map[string]bool, len(env))
	for _, pair := range env {
		key, _, _ := strings.Cut(pair, "=")
		set[key] = true
	}

	for _, key := range terminalEnv {
		if value := os.Getenv(key); value != "" && !set[key] {
			env = append(env, key+"="+value)
		}
	}

	return env
}

// resetTerminal undoes modes a remote program may have left set, so the local
// shell is usable after the session ends.
func resetTerminal(w io.Writer) {
	fmt.Fprint(w,
		"\033[?1004l", // focus reporting off
		"\033[?2004l", // bracketed paste off
		"\033[0m",     // reset colors and attributes
		"\033[?25h",   // show the cursor
		"\r\n",
	)
}

// detachReader passes terminal input through until it sees the detach
// character, then signals detached and ends the stream.
type detachReader struct {
	r        io.Reader
	detached chan struct{}
	once     sync.Once
}

func (d *detachReader) Read(p []byte) (int, error) {
	n, err := d.r.Read(p)

	for i := 0; i < n; i++ {
		if p[i] == detachChar {
			d.once.Do(func() { close(d.detached) })

			// Deliver what was typed before the detach character.
			return i, io.EOF
		}
	}

	return n, err
}

// forwardPorts forwards ports the remote command opens to the same port on
// localhost, and stops forwarding each one when it closes. The returned
// function closes every forward still open.
func forwardPorts(ctx context.Context, cmd *sprites.Cmd, sprite *sprites.Sprite, enabled bool) func() {
	if !enabled {
		return func() {}
	}

	var (
		mu     sync.Mutex
		active = map[string]*sprites.ProxySession{}
	)

	cmd.TextMessageHandler = func(data []byte) {
		var msg sprites.PortNotificationMessage
		if err := json.Unmarshal(data, &msg); err != nil || msg.Port == 0 {
			return
		}

		host := msg.Address
		if host == "" {
			host = "localhost"
		}

		key := fmt.Sprintf("%s:%d", host, msg.Port)

		switch msg.Type {
		case "port_opened":
			mu.Lock()
			_, exists := active[key]
			mu.Unlock()

			if exists {
				return
			}

			// A local port that is already taken is not worth failing the
			// command over; `fly sprites proxy` can map it elsewhere.
			forwards, err := sprite.ProxyPorts(ctx, []sprites.PortMapping{{
				LocalPort:  msg.Port,
				RemotePort: msg.Port,
				RemoteHost: host,
			}})
			if err != nil || len(forwards) == 0 {
				return
			}

			mu.Lock()
			active[key] = forwards[0]
			mu.Unlock()
		case "port_closed":
			mu.Lock()
			forward := active[key]
			delete(active, key)
			mu.Unlock()

			if forward != nil {
				_ = forward.Close()
			}
		}
	}

	return func() {
		mu.Lock()
		defer mu.Unlock()

		for key, forward := range active {
			_ = forward.Close()
			delete(active, key)
		}
	}
}
