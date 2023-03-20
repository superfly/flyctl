package logs

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

func newShipperSetup() (cmd *cobra.Command) {

	const (
		short = "Set up a log shipper VM"
		long  = short + "\n"
	)

	cmd = command.New("setup", short, long, runSetup, command.RequireSession, command.RequireAppName)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)
	return cmd
}

func runSetup(ctx context.Context) (err error) {
	client := client.FromContext(ctx).API().GenqClient
	io := iostreams.FromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	var shipperApp gql.AppData
	var logtailToken string

	if err != nil {
		return err
	}

	// Fetch the target organization from the app
	appNameResponse, err := gql.GetApp(ctx, client, appName)

	if err != nil {
		return err
	}

	targetApp := appNameResponse.App.AppData
	targetOrg := targetApp.Organization
	appsResult, err := gql.GetAppsByRole(ctx, client, "log-shipper", targetOrg.Id)

	if err != nil {
		return err
	}

	if len(appsResult.Apps.Nodes) > 0 {
		shipperApp = appsResult.Apps.Nodes[0].AppData
		fmt.Fprintf(io.ErrOut, "Log shipper already provisioned as app %s\n", shipperApp.Name)
	} else {
		input := gql.DefaultCreateAppInput()
		input.Machines = true
		input.OrganizationId = targetOrg.Id
		input.AppRoleId = "log-shipper"

		createdAppResult, err := gql.CreateApp(ctx, client, input)

		if err != nil {
			return err
		}

		shipperApp = createdAppResult.CreateApp.App.AppData
		fmt.Fprintf(io.ErrOut, "Provisioning a log shipper VM in the app named %s\n", shipperApp.Name)
	}

	// Fetch or create the org-specific Logtail integration

	addOnName := targetOrg.RawSlug + "-log-shipper"

	getAddOnResponse, err := gql.GetAddOn(ctx, client, addOnName)

	if err != nil {
		createAddOnResponse, err := gql.CreateAddOn(ctx, client, targetOrg.Id, "", addOnName, "", nil, "logtail", gql.AddOnOptions{})

		if err != nil {
			return err
		}

		logtailToken = createAddOnResponse.CreateAddOn.AddOn.Token

	} else {
		logtailToken = getAddOnResponse.AddOn.Token
	}
	// Fetch a macaroon token whose access is limited to reading app logs
	tokenResponse, err := gql.CreateLimitedAccessToken(ctx, client, targetOrg.Slug+"-logs", targetOrg.Id, "read_organization_apps", &gql.LimitedAccessTokenOptions{
		"app_id": targetApp.Name,
	})

	if err != nil {
		return
	}

	fmt.Fprintf(io.ErrOut, "Setting ORG, ACCESS_TOKEN and LOGTAIL_TOKEN secrets on %s\n", shipperApp.Name)

	secrets := gql.SetSecretsInput{
		AppId:      shipperApp.Id,
		ReplaceAll: true,
		Secrets: []gql.SecretInput{
			{
				Key:   "ACCESS_TOKEN",
				Value: tokenResponse.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader,
				// Value: flyctl.GetAPIToken(),
			},
			{
				Key:   "LOGTAIL_TOKEN",
				Value: logtailToken,
			},
			{
				Key:   "ORG",
				Value: targetOrg.RawSlug,
			},
			{
				Key:   "VECTOR_WATCH_CONFIG",
				Value: "1",
			},
		},
	}
	_, err = gql.SetSecrets(ctx, client, secrets)

	if err != nil {
		return err
	}

	flapsClient, err := flaps.New(ctx, api.GqlAppForFlaps(shipperApp))

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
		AppID:  shipperApp.Name,
		Name:   "log-shipper",
		Config: machineConf,
	}

	// We already have a log shipper VM, so just update it in-place to pick up any new secrets
	if len(machines) > 0 {
		fmt.Fprintf(io.Out, "Restarting machine %s\n", machines[0].ID)
		machine := machine.NewLeasableMachine(flapsClient, io, machines[0])
		machine.AcquireLease(ctx, time.Second*5)
		launchInput.ID = machines[0].ID
		launchInput.Config = machines[0].Config
		machine.Update(ctx, launchInput)
		machine.ReleaseLease(ctx)
		return
	}

	regionResponse, err := gql.GetNearestRegion(ctx, client)

	if err != nil {
		return err
	}

	launchInput.Region = regionResponse.NearestRegion.Code

	machine, err := flapsClient.Launch(ctx, launchInput)

	fmt.Fprintf(io.Out, "Launched machine %s\n in the %s region", machine.ID, launchInput.Region)

	return
}
