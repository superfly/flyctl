package redis

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/proxy"

	"github.com/superfly/flyctl/internal/command"
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
	cmd.Env = append(cmd.Env, fmt.Sprintf("REDISCLI_AUTH=%s", password))
	cmd.Stdout = io.Out
	cmd.Stderr = io.ErrOut
	cmd.Stdin = io.In

	// Leaving redis-cli with Ctrl-C cancels ctx, and isn't a failure.
	if err := cmd.Run(); err != nil && ctx.Err() == nil {
		return fmt.Errorf("redis-cli failed: %w", err)
	}

	return nil
}
