package synthetics

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/metrics/synthetics"
)

func New() *cobra.Command {
	const (
		short = "Synthetic monitoring"
		long  = `Commands for Fly.io synthetic monitoring. ` + "`fly synthetics agent`" + ` runs the
agent that performs probes.`
	)
	cmd := command.New("synthetics", short, long, nil)
	cmd.AddCommand(
		newAgent(),
	)

	return cmd
}

func newAgent() *cobra.Command {
	const (
		short = "Run the synthetics agent"
		long  = `Run the synthetics agent in the foreground until you interrupt it. The agent
connects to the Fly.io synthetics service over a WebSocket, runs the HTTP
probes it's sent against Fly.io infrastructure (hosts under fly.io,
flyio.net and machines.dev; other targets are skipped), and sends back the
results.`
	)
	cmd := command.New("agent", short, long, runAgent,
		command.RequireSession,
	)
	cmd.Args = cobra.NoArgs

	return cmd
}

func runAgent(ctx context.Context) (err error) {
	err = synthetics.RunAgent(ctx)
	if err != nil {
		return err
	}
	<-ctx.Done()

	return ctx.Err()
}
