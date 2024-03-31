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

	localProxyPort := "16379"

	params, password, err := getRedisProxyParams(ctx, localProxyPort)
	if err != nil {
		return err
	}

	redisCliPath, err := exec.LookPath("redis-cli")
	if err != nil {
		fmt.Fprintf(io.Out, "Could not find redis-cli in your $PATH. Install it or point your redis-cli at: %s", "someurl")
		return
	}

	err = proxy.Start(ctx, params)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, redisCliPath, "-p", localProxyPort)
	cmd.Env = append(cmd.Env, fmt.Sprintf("REDISCLI_AUTH=%s", password))
	cmd.Stdout = io.Out
	cmd.Stderr = io.ErrOut
	cmd.Stdin = io.In

	cmd.Start()
	cmd.Wait()

	return
}
