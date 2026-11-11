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
	"github.com/superfly/flyctl/internal/flag"
)

func newConnect() (cmd *cobra.Command) {
	const (
		short = "Connect to a Redis database using redis-cli"
		long  = short + `. Pass the database name to skip the interactive picker,
for example in scripts. Without a name, you choose from the Redis databases you
can access, narrowed by --org if set.`

		usage = "connect [name]"
	)

	cmd = command.New(usage, short, long, runConnect, command.RequireSession)
	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.Org(),
		flag.Region(),
	)

	return cmd
}

func runConnect(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)

	localProxyPort := "16379"

	// Check for redis-cli before resolving the database so a missing binary
	// fails fast, and with a non-zero exit, instead of after a prompt.
	redisCliPath, err := exec.LookPath("redis-cli")
	if err != nil {
		return errors.New("could not find redis-cli in your $PATH; install it, or run `fly redis proxy` and point your own client at the local port")
	}

	params, password, err := getRedisProxyParams(ctx, localProxyPort)
	if err != nil {
		return err
	}

	err = proxy.Start(ctx, params)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, redisCliPath, "-p", localProxyPort)
	cmd.Env = append(os.Environ(), fmt.Sprintf("REDISCLI_AUTH=%s", password))
	cmd.Stdout = io.Out
	cmd.Stderr = io.ErrOut
	cmd.Stdin = io.In

	err = cmd.Run()

	// Like `fly mpg connect`: redis-cli ending on a signal (Ctrl-C, or the
	// context being cancelled) is a normal way to leave the session.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return nil
		}
	}

	return err
}
