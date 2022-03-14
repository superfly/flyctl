package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newStatus() *cobra.Command {
	const (
		short = "Show current status of a running mchine"
		long  = short + "\n"

		usage = "status <id>"
	)

	cmd := command.New(usage, short, long, runMachineStatus,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func runMachineStatus(ctx context.Context) error {
	return fmt.Errorf("not inplemented")
}
