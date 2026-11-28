package redis

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/proxy"

	"github.com/superfly/flyctl/internal/command"
	tracecmd "github.com/superfly/flyctl/internal/command/trace"
	"github.com/superfly/flyctl/internal/flag"
)

func newConnect() (cmd *cobra.Command) {
	const (
		short = "Connect to a Redis database using redis-cli"
		long  = `Open a redis-cli session to an Upstash Redis database. You pick the database
from a list, so this needs a terminal; pass ` + "`--org`" + ` or set ` + "`FLY_ORG`" + ` to list
only one organization's databases.

The database is proxied to local port 16379 over your organization's
private network, and redis-cli connects to it with the database password
already set. redis-cli must be on your PATH. To use another client, run
` + "`fly redis proxy`" + ` instead.`

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
	case interrupted(ctx, exitErr):
		return nil
	}

	// redis-cli has already printed any error; pass its status through.
	if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return &tracecmd.ExitError{Code: 128 + int(status.Signal())}
	}
	if code := exitErr.ExitCode(); code > 0 {
		return &tracecmd.ExitError{Code: code}
	}

	return fmt.Errorf("redis-cli failed: %w", err)
}

// interruptGrace is how long a failed redis-cli waits for flyctl to see the
// same Ctrl-C.
var interruptGrace = 250 * time.Millisecond

// interrupted reports whether redis-cli ended because of Ctrl-C. The terminal
// sends Ctrl-C to redis-cli and flyctl together. redis-cli either dies of it
// or catches it and exits 1, and it can do either before flyctl cancels ctx,
// so a failed exit waits briefly for that.
func interrupted(ctx context.Context, err *exec.ExitError) bool {
	if status, ok := err.Sys().(syscall.WaitStatus); ok && status.Signaled() && status.Signal() == syscall.SIGINT {
		return true
	}

	select {
	case <-ctx.Done():
		return true
	case <-time.After(interruptGrace):
		return false
	}
}
