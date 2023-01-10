package machine

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
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
		newRemove(),
		newRun(),
		newStart(),
		newStop(),
		newStatus(),
		newProxy(),
		newClone(),
		newUpdate(),
		newRestart(),
		newLeases(),
	)

	return cmd
}

func appFromMachineOrName(ctx context.Context, machineId string, appName string) (app *api.AppCompact, err error) {
	client := client.FromContext(ctx).API()

	if appName == "" {
		machine, err := client.GetMachine(ctx, machineId)
		if err != nil {
			return nil, err
		}
		app = machine.App
	} else {
		app, err = client.GetAppCompact(ctx, appName)
	}

	return app, err
}
