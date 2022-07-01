package agent

import (
	"bytes"
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/render"
)

func newPing() (cmd *cobra.Command) {
	const (
		short = "Ping the Fly agent"
		long  = short + "\n"
	)

	cmd = command.New("ping", short, long, runPing)

	cmd.Args = cobra.NoArgs

	return
}

func runPing(ctx context.Context) (err error) {
	var client *agent.Client
	if client, err = dial(ctx); err != nil {
		return
	}

	var pong agent.PingResponse
	if pong, err = client.Ping(ctx); err != nil {
		err = fmt.Errorf("failed pinging agent: %w", err)

		return
	}

	if out := iostreams.FromContext(ctx).Out; config.FromContext(ctx).JSONOutput {
		err = render.JSON(out, pong)
	} else {
		var buf bytes.Buffer

		fmt.Fprintf(&buf, "%-10s: %d\n", "PID", pong.PID)
		fmt.Fprintf(&buf, "%-10s: %s\n", "Version", pong.Version)
		fmt.Fprintf(&buf, "%-10s: %t\n", "Background", pong.Background)

		_, err = buf.WriteTo(out)
	}

	return
}
