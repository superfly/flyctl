package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newRemove() *cobra.Command {
	const (
		short = "Remove a machine"
		long  = short + "\n"

		usage = "remove"
	)

	cmd := command.New(usage, short, long, runMachineRemove,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runMachineRemove(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}
