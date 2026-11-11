package agent

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func newStart() (cmd *cobra.Command) {
	const (
		short = "Start the Fly agent"
		long  = `Start the Fly agent as a background process. Fails if an agent is already
running; use ` + "`fly agent restart`" + ` to replace it. Requires you to be
logged in.

You rarely need this: commands that use WireGuard start the agent
themselves.
`
	)

	cmd = command.New("start", short, long, runStart,
		command.RequireSession,
	)

	cmd.Args = cobra.NoArgs

	return
}

func runStart(ctx context.Context) error {
	if _, err := dial(ctx); err == nil {
		return errDupInstance
	}

	_, err := establish(ctx)

	return err
}
