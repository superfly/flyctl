package machine

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		short = "Commands that manage machines"
		long  = short + "\n"
		usage = "machine <command>"
	)

	cmd := command.New(usage, short, long, nil)

	cmd.Args = cobra.NoArgs

	cmd.Aliases = []string{"machines", "m"}

	cmd.AddCommand(
		newKill(),
		newList(),
		newDestroy(),
		newRun(),
		newStart(),
		newStop(),
		newStatus(),
		newProxy(),
		newClone(),
		newUpdate(),
		newRestart(),
		newLeases(),
		newMachineExec(),
		newMachineCordon(),
		newMachineUncordon(),
	)

	return cmd
}
