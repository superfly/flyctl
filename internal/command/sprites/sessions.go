package sprites

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	sprites "github.com/superfly/sprites-go"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

// consoleCommand is the login shell wrapper every sprite ships with. It picks
// the shell from the forwarded shell environment, falling back to bash.
const consoleCommand = "/.sprite/bin/sprite-console"

// shellEnv is the local environment the console wrapper uses to pick a shell.
var shellEnv = []string{"BASH_VERSION", "ZSH_VERSION", "FISH_VERSION", "KSH_VERSION", "tcsh", "SHELL"}

func newConsole() *cobra.Command {
	const (
		short = "Open an interactive shell in a sprite."

		long = `Open an interactive shell in a sprite, in a new session with a
pseudo-terminal.

The shell matches your local one when the sprite has it (bash, zsh, fish, tcsh
or ksh), and is bash otherwise. Press Ctrl+\ to detach: the session keeps
running, and 'fly sprites attach' reconnects to it.
`
	)

	cmd := newCommand("console [flags]", short, long, runConsole)
	cmd.Aliases = []string{"c"}
	cmd.Args = cobra.NoArgs

	flag.Add(cmd, noPortForwardFlag("console"))

	return cmd
}

func noPortForwardFlag(what string) flag.Bool {
	return flag.Bool{
		Name:        "no-port-forward",
		Description: "Do not forward ports opened by the " + what + " to localhost",
	}
}

func runConsole(ctx context.Context, s *session) error {
	sprite, err := s.sprite(ctx, "")
	if err != nil {
		return err
	}

	return openConsole(ctx, sprite, !flag.GetBool(ctx, "no-port-forward"))
}

func openConsole(ctx context.Context, sprite *sprites.Sprite, portForward bool) error {
	var env []string

	for _, key := range shellEnv {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}

	env = append(env, shareTerminfo(ctx, sprite)...)

	return runRemote(ctx, sprite, remoteOptions{
		args:        []string{consoleCommand},
		env:         env,
		tty:         true,
		portForward: portForward,
	})
}

func newSessions() *cobra.Command {
	const (
		short = "Manage a sprite's running sessions."

		long = `List, attach to, and kill the sessions running in a sprite. A session is a
command started with 'fly sprites exec' or 'fly sprites console'.
`
	)

	cmd := command.New("sessions", short, long, nil)
	cmd.Aliases = []string{"session", "s"}

	cmd.AddCommand(
		newSessionsList(),
		newSessionsAttach(),
		newSessionsKill(),
	)

	return cmd
}

func newSessionsList() *cobra.Command {
	const (
		short = "List running sessions."
		long  = short + "\n"
	)

	cmd := newCommand("list [flags]", short, long, runSessionsList)
	cmd.Aliases = []string{"ls"}
	cmd.Args = cobra.NoArgs

	flag.Add(cmd, flag.JSONOutput())

	return cmd
}

func runSessionsList(ctx context.Context, s *session) error {
	io := iostreams.FromContext(ctx)

	name, err := s.spriteName(ctx, "")
	if err != nil {
		return err
	}

	sessions, err := s.Client.ListSessions(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	if flag.GetBool(ctx, "json") {
		if sessions == nil {
			sessions = []*sprites.Session{}
		}

		return writeJSON(io.Out, sessions)
	}

	if len(sessions) == 0 {
		fmt.Fprintln(io.Out, "No running sessions.")
		return nil
	}

	return renderSessions(io.Out, sessions)
}

func renderSessions(w io.Writer, sessions []*sprites.Session) error {
	rows := make([][]string, len(sessions))

	for i, session := range sessions {
		activity := "idle"
		if session.IsActive {
			activity = "active"
		}

		rows[i] = []string{
			session.ID,
			session.Command,
			yesNo(session.TTY),
			formatAge(time.Since(session.Created)) + " ago",
			activity,
		}
	}

	return render.Table(w, "", rows, "ID", "Command", "TTY", "Created", "Activity")
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}

	return "no"
}

func newSessionsAttach() *cobra.Command {
	const (
		short = "Attach to a running session by ID."
		long  = short + "\n"
	)

	cmd := newCommand("attach [flags] <session-id>", short, long, runSessionsAttach)
	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd, noPortForwardFlag("session"))

	return cmd
}

func runSessionsAttach(ctx context.Context, s *session) error {
	sprite, err := s.sprite(ctx, "")
	if err != nil {
		return err
	}

	sessions, err := s.Client.ListSessions(ctx, sprite.Name())
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	id := flag.FirstArg(ctx)

	for _, session := range sessions {
		if session.ID == id {
			return attachSession(ctx, sprite, session)
		}
	}

	return flyerr.WithSuggestion(
		fmt.Errorf("no session with ID %s", id),
		"Run 'fly sprites sessions list' to see running sessions.",
	)
}

// attachSession connects the terminal to a running session.
func attachSession(ctx context.Context, sprite *sprites.Sprite, session *sprites.Session) error {
	opts := remoteOptions{
		sessionID:     session.ID,
		tty:           session.TTY,
		portForward:   !flag.GetBool(ctx, "no-port-forward"),
		detachMessage: fmt.Sprintf("Detached from session %s.", session.ID),
	}

	if !session.TTY {
		// A non-TTY session has no screen to redraw, so replay its output.
		opts.replay = true

		// Watching a session must not end its input. The SDK tells the
		// session its input is over both when given no stdin and when the
		// stdin it is given ends, so give it one that never ends.
		opts.stdin = openEndedReader{os.Stdin}
	}

	return runRemote(ctx, sprite, opts)
}

func newSessionsKill() *cobra.Command {
	const (
		short = "Kill a running session."

		long = `Kill a running session. The session is sent --signal, then SIGKILL if it is
still running after --timeout.
`
	)

	cmd := newCommand("kill [flags] <session-id>", short, long, runSessionsKill)
	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "signal",
			Description: "Signal to send",
			Default:     "TERM",
		},
		flag.Duration{
			Name:        "timeout",
			Description: "How long to wait before sending SIGKILL",
			Default:     10 * time.Second,
		},
	)

	return cmd
}

func runSessionsKill(ctx context.Context, s *session) error {
	out := iostreams.FromContext(ctx).Out

	name, err := s.spriteName(ctx, "")
	if err != nil {
		return err
	}

	query := url.Values{}
	query.Set("signal", strings.TrimPrefix(strings.ToUpper(flag.GetString(ctx, "signal")), "SIG"))
	query.Set("timeout", flag.GetDuration(ctx, "timeout").String())

	path := fmt.Sprintf("/v1/sprites/%s/exec/%s/kill?%s",
		url.PathEscape(name), url.PathEscape(flag.FirstArg(ctx)), query.Encode())

	resp, err := s.request(ctx, http.MethodPost, path)
	if err != nil {
		return fmt.Errorf("failed to kill session: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to kill session: %w", responseError(resp))
	}

	return printKillProgress(out, resp.Body)
}

// printKillProgress relays the API's progress as it signals and reaps a
// session. The API streams one JSON object per line.
func printKillProgress(out io.Writer, body io.Reader) error {
	scanner := bufio.NewScanner(body)

	for scanner.Scan() {
		line := scanner.Text()

		var event struct {
			Type     string `json:"type"`
			Message  string `json:"message"`
			ExitCode *int   `json:"exit_code"`
		}

		switch err := json.Unmarshal([]byte(line), &event); {
		case err != nil:
			// Not the format this was written against: show it as is.
			fmt.Fprintln(out, line)
		case event.Type == "error":
			return fmt.Errorf("failed to kill session: %s", event.Message)
		case event.Message != "":
			fmt.Fprintln(out, event.Message)
		case event.ExitCode != nil:
			fmt.Fprintf(out, "Session ended with exit status %d\n", *event.ExitCode)
		}
	}

	return scanner.Err()
}

func newAttach() *cobra.Command {
	const (
		short = "Attach to a running session."

		long = `Attach to a running session, by session ID or by the command it is running.

Given a command, this attaches to the oldest session whose command starts with
it. Given nothing, it lists the sessions to pick from, and offers to open a
console when there are none.
`
	)

	cmd := newCommand("attach [flags] [session-id|command]", short, long, runAttach)
	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Example = `  fly sprites attach             # pick a session
  fly sprites attach 3           # attach to session 3
  fly sprites attach bash        # attach to the oldest session running bash`

	flag.Add(cmd, noPortForwardFlag("session"))

	return cmd
}

func runAttach(ctx context.Context, s *session) error {
	io := iostreams.FromContext(ctx)

	sprite, err := s.sprite(ctx, "")
	if err != nil {
		return err
	}

	sessions, err := s.Client.ListSessions(ctx, sprite.Name())
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	target := flag.FirstArg(ctx)

	if len(sessions) == 0 {
		noSessions := flyerr.WithSuggestion(
			fmt.Errorf("no running sessions in sprite %s", sprite.Name()),
			"Start one with 'fly sprites console'.",
		)

		if target != "" {
			return noSessions
		}

		switch start, err := prompt.Confirm(ctx, "No running sessions. Start a new console session?"); {
		case prompt.IsNonInteractive(err), err == nil && !start:
			return noSessions
		case err != nil:
			return err
		}

		return openConsole(ctx, sprite, !flag.GetBool(ctx, "no-port-forward"))
	}

	session, err := pickSession(ctx, sessions, target)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.ErrOut, "Attaching to session %s (%s)...\n", session.ID, session.Command)

	return attachSession(ctx, sprite, session)
}

// pickSession finds the session target names: an exact session ID, else the
// oldest session whose command starts with target. An empty target prompts.
func pickSession(ctx context.Context, sessions []*sprites.Session, target string) (*sprites.Session, error) {
	if target == "" {
		options := make([]string, len(sessions))
		for i, session := range sessions {
			options[i] = fmt.Sprintf("%s  %s  (started %s ago)",
				session.ID, session.Command, formatAge(time.Since(session.Created)))
		}

		var index int
		if err := prompt.Select(ctx, &index, "Select a session:", "", options...); err != nil {
			if prompt.IsNonInteractive(err) {
				return nil, errors.New("a session ID or command is required when not running interactively")
			}

			return nil, err
		}

		return sessions[index], nil
	}

	var matches []*sprites.Session

	for _, session := range sessions {
		if session.ID == target {
			return session, nil
		}

		if strings.HasPrefix(session.Command, target) {
			matches = append(matches, session)
		}
	}

	if len(matches) == 0 {
		return nil, flyerr.WithSuggestion(
			fmt.Errorf("no session with an ID or command matching %q", target),
			"Run 'fly sprites sessions list' to see running sessions.",
		)
	}

	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].Created.Before(matches[j].Created)
	})

	return matches[0], nil
}

// openEndedReader passes input through and, when it runs out, waits forever
// instead of reporting the end.
type openEndedReader struct {
	r io.Reader
}

func (o openEndedReader) Read(p []byte) (int, error) {
	n, err := o.r.Read(p)
	if n > 0 || err == nil {
		return n, nil
	}

	select {}
}
