package machine

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		short = "Manage Fly Machines"
		long  = `Manage the Fly Machines (VMs) that make up an app: list them, create and
run new ones, change their configuration, and start, stop or destroy them.

Most subcommands act on the app from ` + "`--app`" + ` or fly.toml. Commands that
take machine IDs let you pick machines from a list when you run them in a
terminal without IDs. Start with ` + "`fly machine list`" + ` or
` + "`fly machine run`" + `.`
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
		newWait(),
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
		newPlace(),
	)

	return cmd
}
