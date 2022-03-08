package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newStart() *cobra.Command {
	const (
		short = "Start a machine"
		long  = short + "\n"

		usage = "start"
	)

	cmd := command.New(usage, short, long, runMachineStart,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runMachineStart(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}
