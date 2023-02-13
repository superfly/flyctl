package agent

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func newRestart() (cmd *cobra.Command) {
	const (
		short = "Restart the Fly agent"
		long  = short + "\n"
	)

	cmd = command.New("restart", short, long, runRestart,
		command.RequireSession,
	)

	cmd.Args = cobra.NoArgs

	return
}

func runRestart(ctx context.Context) error {
	if client, err := dial(ctx); err == nil {
		_ = client.Kill(ctx)
	}

	_, err := establish(ctx)
	return err
}
