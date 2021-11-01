package apps

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newRestart() *cobra.Command {
	restart := command.FromDocstrings("apps.restart", runRestart,
		command.RequireSession)

	restart.Args = cobra.RangeArgs(0, 1)

	return restart
}

func runRestart(ctx context.Context) error {
	return command.ErrNotImplementedYet
}
