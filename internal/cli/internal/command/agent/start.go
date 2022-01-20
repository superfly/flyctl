package agent

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newStart() (cmd *cobra.Command) {
	const (
		short = "Start the Fly agent"
		long  = short + "\n"
	)

	cmd = command.New("start", short, long, runStart,
		command.RequireSession,
	)

	cmd.Args = cobra.NoArgs

	return
}

func runStart(ctx context.Context) error {
	if client, err := dial(ctx); err == nil {
		_ = client.Kill(ctx)
	}

	_, err := establish(ctx)
	return err
}
