package machine

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		short = "Manage Fly Machines."
		long  = short + ` Fly Machines are super-fast, lightweight VMs that can be created,
and then quickly started and stopped as needed with flyctl commands or with the
Machines REST fly.`
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
		newCreate(),
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
		newSuspend(),
		newEgressIp(),
	)

	return cmd
}
