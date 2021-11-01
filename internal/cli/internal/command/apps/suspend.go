package apps

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newSuspend() *cobra.Command {
	suspend := command.FromDocstrings("apps.suspend", runSuspend,
		command.RequireSession)

	suspend.Args = cobra.RangeArgs(0, 1)

	return suspend
}

func runSuspend(ctx context.Context) error {
	return command.ErrNotImplementedYet
}
