package sprites

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	sprites "github.com/superfly/sprites-go"
	"golang.org/x/sync/errgroup"
	"golang.org/x/term"

	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/iostreams"
)

// detachChar is Ctrl+\. Typing it in a TTY session disconnects and leaves the
// session running on the sprite.
const detachChar = 0x1C

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
			Description: "Environment variables as KEY=value, comma-separated or repeated",
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
		env:         splitEnv(flag.GetStringArray(ctx, "env")),
		tty:         tty,
		portForward: !flag.GetBool(ctx, "no-port-forward"),
	}

	// Forward stdin when asked to, or when something is piped in.
	if !tty && !noStdin && (stdin || !io.IsStdinTTY()) {
		opts.stdin = io.In
	}

	return runRemote(ctx, sprite, opts)
}

// splitEnv flattens --env values, each of which may hold several
// comma-separated KEY=value pairs.
func splitEnv(values []string) []string {
	var env []string

	for _, value := range values {
		for _, pair := range strings.Split(value, ",") {
			if pair = strings.TrimSpace(pair); pair != "" {
				env = append(env, pair)
			}
		}
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
		source, dest, ok := strings.Cut(value, ":")
		if !ok || source == "" || dest == "" {
			return nil, fmt.Errorf("invalid --file %q: expected source:dest", value)
		}

		specs = append(specs, fileSpec{source: source, dest: dest})
	}

	return specs, nil
}

// uploadFiles writes local files into the sprite before a command runs.
// Relative destinations are resolved against dir.
func uploadFiles(ctx context.Context, io *iostreams.IOStreams, sprite *sprites.Sprite, dir string, files []fileSpec) error {
	if len(files) == 0 {
		return nil
	}

	if dir == "" {
		dir = "/"
	}

	fsys := sprite.FilesystemAt(dir)

	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)

	for _, file := range files {
		g.Go(func() error {
			data, err := os.ReadFile(file.source)
			if err != nil {
				return fmt.Errorf("failed to read %s: %w", file.source, err)
			}

			if err := fsys.WriteFileContext(gctx, file.dest, data, 0o644); err != nil {
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

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var cmd *sprites.Cmd
	if opts.sessionID != "" {
		cmd = sprite.AttachSessionContext(ctx, opts.sessionID)
	} else {
		cmd = sprite.CommandContext(ctx, opts.args[0], opts.args[1:]...)
		cmd.Dir = opts.dir
	}

	env := opts.env
	if opts.tty {
		env = withTerminalEnv(env)
	}

	if len(env) > 0 {
		cmd.Env = env
	}

	stopForwarding := forwardPorts(ctx, cmd, sprite, opts.portForward)
	defer stopForwarding()

	cmd.Stdout = io.Out
	cmd.Stderr = io.ErrOut

	stdinFd := int(os.Stdin.Fd())
	interactive := opts.tty && term.IsTerminal(stdinFd)

	detached := make(chan struct{})

	switch {
	case opts.tty:
		cmd.SetTTY(true)

		if interactive {
			if width, height, err := term.GetSize(stdinFd); err == nil {
				_ = cmd.SetTTYSize(uint16(height), uint16(width))
			}
		}

		cmd.Stdin = &detachReader{r: os.Stdin, detached: detached}
	case opts.sessionID != "":
		if opts.replay {
			cmd.SetOutputOffset(0)
		}

		cmd.Stdin = opts.stdin
	default:
		cmd.Stdin = opts.stdin
	}

	if err := cmd.Start(); err != nil {
		if ctx.Err() != nil {
			return &flyerr.ExitCodeError{Code: 130}
		}

		return fmt.Errorf("failed to start command: %w", err)
	}

	restore := func() {}

	if interactive {
		oldState, err := term.MakeRaw(stdinFd)
		if err != nil {
			return fmt.Errorf("failed to set the terminal to raw mode: %w", err)
		}

		var once sync.Once
		restore = func() {
			once.Do(func() {
				resetTerminal(io.Out)
				_ = term.Restore(stdinFd, oldState)
			})
		}
		defer restore()

		// The size given before the start is not applied to the new terminal,
		// so send it again now that the command is running.
		if width, height, err := term.GetSize(stdinFd); err == nil && width > 0 && height > 0 {
			_ = cmd.SetTTYSize(uint16(height), uint16(width))
		}

		stopResize := watchResize(stdinFd, func(rows, cols uint16) {
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
	case err := <-waitDone:
		restore()

		var exitErr *sprites.ExitError

		switch {
		case err == nil:
			return nil
		case errors.As(err, &exitErr):
			return &flyerr.ExitCodeError{Code: exitErr.ExitCode()}
		case ctx.Err() != nil:
			// Interrupted locally, as a shell would report it.
			return &flyerr.ExitCodeError{Code: 130}
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
