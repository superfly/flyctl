package agent

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func newRestart() (cmd *cobra.Command) {
	const (
		short = "Restart the Fly agent"
		long  = `Stop the running Fly agent, if there is one, and start a new one in the
background. Restarting closes the agent's open WireGuard tunnels, so any
` + "`fly proxy`" + ` or ` + "`fly ssh`" + ` sessions using it are interrupted. Requires
you to be logged in.
`
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
