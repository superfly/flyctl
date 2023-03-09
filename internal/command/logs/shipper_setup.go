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
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

func newShipperSetup() (cmd *cobra.Command) {

	const (
		short = "Set up a log shipper VM"
		long  = short + "\n"
	)

	cmd = command.New("setup", short, long, runSetup, command.RequireSession)

	return cmd
}

func runSetup(ctx context.Context) (err error) {
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
			AppRoleID:      "log_shipper",
		}

		createdApp, err := client.CreateApp(ctx, input)

		if err != nil {
			return err
		}

		app = client.AppToCompact(createdApp)

	}

	response, err := gql.GetAddOn(ctx, client.GenqClient, appName)
	var token string

	if err != nil {

		fmt.Fprintf(io.ErrOut, "Provisioning log shipper VM as the app named %s\n", app.Name)

		response, err := gql.CreateAddOn(ctx, client.GenqClient, selectedOrg.ID, "", appName, "", nil, "logtail", api.AddOnOptions{})

		if err != nil {
			return err
		}

		token = response.CreateAddOn.AddOn.Name
	} else {
		token = response.AddOn.Token
	}

	fmt.Fprintf(io.ErrOut, "Setting up secrets for %s\n", app.Name)

	_, err = client.SetSecrets(ctx, appName, map[string]string{
		"ACCESS_TOKEN":  flyctl.GetAPIToken(),
		"LOGTAIL_TOKEN": token,
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
