package apps

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newResume() *cobra.Command {
	resume := command.FromDocstrings("apps.resume", runResume,
		command.RequireSession)

	resume.Args = cobra.RangeArgs(0, 1)

	return resume
}

func runResume(ctx context.Context) error {
	return command.ErrNotImplementedYet
}
