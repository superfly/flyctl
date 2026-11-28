package redis

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/proxy"

	"github.com/superfly/flyctl/internal/command"
	tracecmd "github.com/superfly/flyctl/internal/command/trace"
	"github.com/superfly/flyctl/internal/flag"
)

func newConnect() (cmd *cobra.Command) {
	const (
		long = `Connect to a Redis database using redis-cli`

		short = long
		usage = "connect"
	)

	cmd = command.New(usage, short, long, runConnect, command.RequireSession)

	flag.Add(cmd,
		flag.Org(),
		flag.Region(),
	)

	return cmd
}

func runConnect(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)

	redisCliPath, err := exec.LookPath("redis-cli")
	if err != nil {
		return fmt.Errorf("could not find redis-cli in your $PATH; install it, or run `fly redis proxy` to use another client: %w", err)
	}

	localProxyPort := "16379"

	params, password, err := getRedisProxyParams(ctx, localProxyPort)
	if err != nil {
		return err
	}

	err = proxy.Start(ctx, params)
	if err != nil {
		return err
	}

	return runRedisCLI(ctx, io, redisCliPath, localProxyPort, password)
}

func runRedisCLI(ctx context.Context, io *iostreams.IOStreams, path, port, password string) error {
	cmd := exec.CommandContext(ctx, path, "-p", port)
	// Keep HOME, TERM and the rest: redis-cli needs HOME for its history.
	cmd.Env = append(os.Environ(), fmt.Sprintf("REDISCLI_AUTH=%s", password))
	cmd.Stdout = io.Out
	cmd.Stderr = io.ErrOut
	cmd.Stdin = io.In

	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil, ctx.Err() != nil:
		// Leaving redis-cli with Ctrl-C cancels ctx, and isn't a failure.
		return nil
	case !errors.As(err, &exitErr):
		return fmt.Errorf("could not run redis-cli: %w", err)
	case interrupted(exitErr):
		// Ctrl-C can reach redis-cli before flyctl cancels ctx.
		return nil
	case exitErr.ExitCode() > 0:
		// redis-cli has already printed its error; pass its status through.
		return &tracecmd.ExitError{Code: exitErr.ExitCode()}
	default:
		return fmt.Errorf("redis-cli failed: %w", err)
	}
}

// interrupted reports whether redis-cli ended because of Ctrl-C.
func interrupted(err *exec.ExitError) bool {
	status, ok := err.Sys().(syscall.WaitStatus)

	return ok && status.Signaled() && status.Signal() == syscall.SIGINT
}
