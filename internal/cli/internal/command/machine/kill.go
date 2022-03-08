package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newKill() *cobra.Command {
	const (
		short = "Kill a machine"
		long  = short + "\n"

		usage = "kill"
	)

	cmd := command.New(usage, short, long, runMachineKill,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runMachineKill(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}
