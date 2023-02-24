package logs

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/machine"
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

	var app *api.AppCompact

	app, err = client.GetAppCompact(ctx, appName)

	if err != nil {
		input := api.CreateAppInput{
			Name:           appName,
			OrganizationID: selectedOrg.ID,
			Machines:       true,
		}

		createdApp, err := client.CreateApp(ctx, input)
		app = client.AppToCompact(createdApp)

		if err != nil {
			return err
		}
	}

	fmt.Fprintf(io.ErrOut, "Setting up secrets for %s\n", app.Name)

	_, err = client.SetSecrets(ctx, appName, map[string]string{
		"ACCESS_TOKEN": flyctl.GetAPIToken(),
	})

	if err != nil {
		return err
	}

	flapsClient, err := flaps.New(ctx, app)

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

	launchInput := api.LaunchMachineInput{
		AppID:  app.Name,
		Name:   "log-shipper",
		Config: machineConf,
	}

	// We already have a log shipper VM, so just update it in-place to pick up any new secrets
	if len(machines) > 0 {
		machine := machine.NewLeasableMachine(flapsClient, io, machines[0])
		machine.AcquireLease(ctx, time.Second*5)
		launchInput.ID = machines[0].ID
		launchInput.Config = machines[0].Config
		machine.Update(ctx, launchInput)
		machine.ReleaseLease(ctx)
		return
	}

	region, err := client.GetNearestRegion(ctx)

	if err != nil {
		return err
	}

	launchInput.Region = region.Code

	machine, err := flapsClient.Launch(ctx, launchInput)

	fmt.Fprintf(io.Out, "Launched machine %s\n", machine.ID)
	return err
}
