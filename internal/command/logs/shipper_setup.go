package logs

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
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
		input.Name = targetOrg.RawSlug + "-log-shipper"

		createdAppResult, err := gql.CreateApp(ctx, client, input)

		if err != nil {
			return err
		}

		shipperApp = createdAppResult.CreateApp.App.AppData
		fmt.Fprintf(io.ErrOut, "Provisioning a log shipper VM in the app named %s\n", shipperApp.Name)
	}

	// Fetch or create the Logtail integration for the app

	var addOnName = appName + "-log-shipper"
	getAddOnResponse, err := gql.GetAddOn(ctx, client, addOnName)

	if err != nil {

		input := gql.CreateAddOnInput{
			OrganizationId: targetOrg.Id,
			Name:           addOnName,
			AppId:          targetApp.Id,
			Type:           "logtail",
		}

		createAddOnResponse, err := gql.CreateAddOn(ctx, client, input)

		if err != nil {
			return err
		}

		logtailToken = createAddOnResponse.CreateAddOn.AddOn.Token

	} else {
		logtailToken = getAddOnResponse.AddOn.Token
	}
	// Fetch a macaroon token whose access is limited to reading this app's logs
	tokenResponse, err := gql.CreateLimitedAccessToken(ctx, client, appName+"-logs", targetOrg.Id, "read_organization_apps", &gql.LimitedAccessTokenOptions{
		"app_id": targetApp.Name,
	}, "")

	if err != nil {
		return
	}

	flapsClient, err := flaps.New(ctx, gql.AppForFlaps(shipperApp))

	if err != nil {
		return err
	}

	machines, err := flapsClient.List(ctx, "")

	if err != nil {
		return err
	}

	var machine *api.Machine

	if len(machines) > 0 {
		machine = machines[0]

	} else {

		machineConf := &api.MachineConfig{
			Guest: &api.MachineGuest{
				CPUKind:  "shared",
				CPUs:     1,
				MemoryMB: 256,
			},
			Image: "flyio/log-shipper:auto-d5a96e6",
		}

		launchInput := api.LaunchMachineInput{
			AppID:  shipperApp.Name,
			Name:   "log-shipper",
			Config: machineConf,
		}

		regionResponse, err := gql.GetNearestRegion(ctx, client)

		if err != nil {
			return err
		}

		launchInput.Region = regionResponse.NearestRegion.Code

		machine, err := flapsClient.Launch(ctx, launchInput)

		if err != nil {
			return err
		}

		fmt.Fprintf(io.Out, "Launched log shipper VM %s\n in the %s region", machine.ID, launchInput.Region)
	}

	cmd := []string{"/add-logger.sh", targetApp.Name, "logtail", "'" + tokenResponse.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader + "'", logtailToken}
	fmt.Println(tokenResponse.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader)
	request := &api.MachineExecRequest{
		Cmd: strings.Join(cmd, " "),
	}

	response, err := flapsClient.Exec(ctx, machine.ID, request)

	if err != nil {
		fmt.Fprintf(io.ErrOut, response.StdErr)
		return err
	}
	return
}
