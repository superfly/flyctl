package extensions

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/secrets"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newPlanetscaleCreate() (cmd *cobra.Command) {

	const (
		short = "Provision a Sentry project for a Fly.io app"
		long  = short + "\n"
	)

	cmd = command.New("create", short, long, runPlanetscaleCreate, command.RequireSession, command.RequireAppName)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)
	return cmd
}

func runPlanetscaleCreate(ctx context.Context) (err error) {
	client := client.FromContext(ctx).API().GenqClient
	io := iostreams.FromContext(ctx)
	appName := appconfig.NameFromContext(ctx)

	if err != nil {
		return err
	}

	// Fetch the target organization from the app
	appResponse, err := gql.GetApp(ctx, client, appName)

	if err != nil {
		return err
	}

	targetApp := appResponse.App.AppData
	targetOrg := targetApp.Organization

	if err != nil {
		return err
	}

	// Fetch or create the Logtail integration for the app

	_, err = gql.GetAddOn(ctx, client, appName)

	if err != nil {

		input := gql.CreateAddOnInput{
			OrganizationId: targetOrg.Id,
			Name:           appName,
			AppId:          targetApp.Id,
			Type:           "planetscale",
		}

		createAddOnResponse, err := gql.CreateAddOn(ctx, client, input)

		if err != nil {
			return err
		}

		env := make(map[string]string)
		for key, value := range createAddOnResponse.CreateAddOn.AddOn.Environment.(map[string]interface{}) {
			env[key] = value.(string)
		}

		fmt.Fprintf(io.Out, "%+v", env)

		secrets.SetSecretsAndDeploy(ctx, gql.ToAppCompact(targetApp), env, false, false)

		return nil
	} else {
		fmt.Fprintln(io.Out, "A Planetscale project already exists for this app")
	}

	return
}
