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
		long  = `Synthetic monitoring management.`
	)
	cmd := command.New("synthetics", short, long, nil)
	cmd.AddCommand(
		newAgent(),
	)
	return cmd
}

func newAgent() *cobra.Command {
	const (
		short = "Runs the Synthetics agent"
		long  = "Runs the Synthetics agent in the foreground."
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
