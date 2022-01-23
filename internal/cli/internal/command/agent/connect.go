package agent

import (
	"context"
	"io"
	"net"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
)

func newConnect() (cmd *cobra.Command) {
	const (
		short = "Connect"
		long  = short + "\n"
		usage = "connect <slug> <addr>"
	)

	cmd = command.New(usage, short, long, runConnect,
		command.RequireSession,
	)

	cmd.Args = cobra.ExactArgs(2)

	return
}

func runConnect(ctx context.Context) (err error) {
	var client *agent.Client
	if client, err = establish(ctx); err != nil {
		return
	}

	var dialer agent.Dialer
	if dialer, err = client.Dialer(ctx, flag.FirstArg(ctx)); err != nil {
		return
	}

	var conn net.Conn
	if conn, err = dialer.DialContext(ctx, "tcp", flag.Args(ctx)[1]); err != nil {
		return
	}
	defer conn.Close()

	out := iostreams.FromContext(ctx).Out
	_, err = io.Copy(out, conn)

	return
}
