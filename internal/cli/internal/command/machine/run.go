package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newRun() *cobra.Command {
	const (
		short = "Run a machine"
		long  = short + "\n"

		usage = "run"
	)

	cmd := command.New(usage, short, long, runMachineRun,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runMachineRun(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}
