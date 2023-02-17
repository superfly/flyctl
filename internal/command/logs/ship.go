package logs

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newShip() *cobra.Command {

	const (
		short = "Ship logs to an external provider"
		long  = short + "\n"
		usage = "ship"
	)

	cmd := command.New(usage, short, long, runShip, command.RequireSession)

	flag.Add(cmd)

	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runShip(ctx context.Context) (err error) {
	client := client.FromContext(ctx).API()
	io := iostreams.FromContext(ctx)
	selectedOrg, err := orgs.OrgFromFirstArgOrSelect(ctx)

	if err != nil {
		return err
	}

	appName := selectedOrg.Slug + "-auto-log-shipper"

	_, err = client.GetAppCompact(ctx, appName)

	if err == nil {
		return
	}

	input := api.CreateAppInput{
		Name:           appName,
		OrganizationID: selectedOrg.ID,
	}

	app, err := client.CreateApp(ctx, input)

	if err != nil {
		return err
	}

	if err != nil {
		return err
	}

	flapsClient, err := flaps.New(ctx, client.AppToCompact(app))

	if err != nil {
		return err
	}
	machineConf := &api.MachineConfig{
		Guest: &api.MachineGuest{
			CPUKind:  "shared",
			CPUs:     1,
			MemoryMB: 256,
		},
		Image: "flyio/log-shipper",
	}

	machines, err := flapsClient.List(ctx, "")

	if len(machines) > 0 {
		return
	}

	region, err := client.GetNearestRegion(ctx)

	if err != nil {
		return err
	}

	launchInput := api.LaunchMachineInput{
		AppID:  app.Name,
		Name:   "log-shipper",
		Region: region.Code,
		Config: machineConf,
	}

	machine, err := flapsClient.Launch(ctx, launchInput)

	fmt.Fprintf(io.Out, "Launched machine %s\n", machine.ID)
	return err
}
