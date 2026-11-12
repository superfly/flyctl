package mpgutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/pflag"
	"github.com/superfly/flyctl/internal/ctrlc"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

// PsqlOptions describes SQL input and arguments passed through to psql.
type PsqlOptions struct {
	Commands  []string
	Files     []string
	ExtraArgs []string
}

// PsqlOptionsFromContext reads the SQL flags and arguments passed through to psql.
func PsqlOptionsFromContext(ctx context.Context) PsqlOptions {
	fs := flag.FromContext(ctx)
	// Read the slice directly: pflag's GetStringArray round-trip loses a single
	// empty command, which is still a valid request to run psql and exit.
	readSlice := func(name string) []string {
		if f := fs.Lookup(name); f != nil {
			if values := f.Value.(pflag.SliceValue).GetSlice(); len(values) > 0 {
				return values
			}
		}

		return nil
	}

	// Use Cobra's parsed dash position rather than flag.ExtraArgsFromContext:
	// global flags may precede the command, and "--" may be a flag value.
	var extra []string
	if n := fs.ArgsLenAtDash(); n >= 0 {
		extra = fs.Args()[n:]
	}

	return PsqlOptions{
		Commands:  readSlice("command"),
		Files:     readSlice("file"),
		ExtraArgs: extra,
	}
}

// Scripted reports whether SQL was supplied with --command or --file.
func (o PsqlOptions) Scripted() bool {
	return len(o.Commands) > 0 || len(o.Files) > 0
}

func (o PsqlOptions) args(url string) []string {
	if !o.Scripted() && len(o.ExtraArgs) == 0 {
		return []string{url}
	}
	args := []string{"--dbname", url}
	if o.Scripted() {
		args = append(args, "-v", "ON_ERROR_STOP=1", "-X")
	}
	for _, sql := range o.Commands {
		args = append(args, "-c", sql)
	}
	for _, file := range o.Files {
		args = append(args, "-f", file)
	}

	return append(args, o.ExtraArgs...)
}

// PsqlProxyContext keeps diagnostics off psql's stdout for non-interactive or scripted sessions.
func PsqlProxyContext(ctx context.Context, o PsqlOptions) context.Context {
	if !o.Scripted() && iostreams.FromContext(ctx).IsInteractive() {
		return ctx
	}
	// The proxy outlives startup; keep psql output and prompt settings unchanged.
	streams := *iostreams.FromContext(ctx)
	streams.Out = streams.ErrOut

	return iostreams.NewContext(ctx, &streams)
}

// RunPsql runs psql against connectURL and waits for it. Interrupts are
// forwarded to psql so Ctrl+C cancels the running query; a second one within
// two seconds ends the session.
func RunPsql(ctx context.Context, psqlPath, connectURL string, options PsqlOptions) error {
	io := iostreams.FromContext(ctx)
	// Allow Ctrl+C signals to hit psql
	psqlCtx, psqlCancel := context.WithCancelCause(context.WithoutCancel(ctx))
	defer psqlCancel(fmt.Errorf("psql session finished: %w", context.Canceled))

	cmd := exec.CommandContext(psqlCtx, psqlPath, options.args(connectURL)...)
	cmd.Stdout = io.Out
	cmd.Stderr = io.ErrOut
	cmd.Stdin = io.In

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	err := cmd.Start()
	if err != nil {
		return err
	}

	done := make(chan struct{})

	go func() {
		var lastSigTime time.Time

		for {
			select {
			case sig := <-sigChan:
				now := time.Now()

				if cmd.Process != nil {
					// Double Ctrl+C — kill the process
					if !lastSigTime.IsZero() && now.Sub(lastSigTime) < 2*time.Second {
						cmd.Process.Kill()
						psqlCancel(ctrlc.AbortedByUser)

						return
					}

					// Forward to psql for query cancellation
					cmd.Process.Signal(sig)
					lastSigTime = now
				}
			case <-done:
				return
			}
		}
	}()

	// A terminal session ends quietly when psql is killed, even when prompts
	// are disabled (FLY_NO_PROMPT), so this checks the terminal itself.
	onTerminal := io.IsStdinTTY() && io.IsStdoutTTY()

	return waitPsql(cmd, done, onTerminal, options.Scripted())
}

// waitPsql classifies psql's result from its reaped process state.
func waitPsql(cmd *exec.Cmd, done chan struct{}, onTerminal, scripted bool) error {
	err := cmd.Wait()
	close(done)
	if state := cmd.ProcessState; state != nil {
		ws, _ := state.Sys().(syscall.WaitStatus)

		return psqlResult(state.Success(), ws.Signaled(), onTerminal, scripted, err)
	}

	return err
}

// psqlResult maps how psql ended to the command's result. err is what Wait
// returned.
func psqlResult(success, signaled, onTerminal, scripted bool, err error) error {
	switch {
	case success && errors.Is(err, context.Canceled):
		// Our cancellation raced with a clean exit; psql succeeded.
		return nil
	case signaled && onTerminal && !scripted:
		// Ending an interactive session with a signal is not a failure.
		return nil
	default:
		return err
	}
}
