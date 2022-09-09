package machine

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
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
		newLaunch(),
		newClone(),
		newUpdate(),
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

func Restart(ctx context.Context, instance *api.Machine) (err error) {
	var (
		flapsClient = flaps.FromContext(ctx)
	)

	if err = Stop(ctx, instance.ID, "0", 50); err != nil {
		return
	}

	if err = flapsClient.Wait(ctx, instance, "stopped"); err != nil {
		return
	}

	if err = Start(ctx, instance.ID); err != nil {
		return
	}

	if err = flapsClient.Wait(ctx, instance, "started"); err != nil {
		return
	}
	return
}
