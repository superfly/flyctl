package agent

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newHex() (cmd *cobra.Command) {
	const (
		short = "hex"
		long  = short + "\n"
		usage = "hex"
	)

	cmd = command.New(usage, short, long, runHex)

	cmd.Args = cobra.NoArgs

	return
}

func runHex(ctx context.Context) (err error) {
	var client *agent.Client
	if client, err = establish(ctx); err != nil {
		return
	}

	var dialer agent.Dialer
	if dialer, err = client.Dialer(ctx, "soupedup"); err != nil {
		return
	}

	var conn net.Conn
	if conn, err = dialer.DialContext(ctx, "tcp", "dumptcp.internal:10000"); err != nil {
		return
	}
	defer conn.Close()

	out := iostreams.FromContext(ctx).Out

	buf := make([]byte, 10)
	for {
		var n int
		if n, err = io.ReadFull(conn, buf); n > 0 {
			fmt.Fprintf(out, "%s\n", buf[:n])
		}

		if err != nil {
			break
		}
	}

	return
}
