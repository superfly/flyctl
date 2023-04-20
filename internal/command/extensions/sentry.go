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

func newSentry() (cmd *cobra.Command) {

	const (
		short = "Setup a Sentry project for this app"
		long  = short + "\n"
	)

	cmd = command.New("sentry", short, long, runSentry, command.RequireSession, command.RequireAppName)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)
	return cmd
}

func runSentry(ctx context.Context) (err error) {
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
			Type:           "sentry",
		}

		createAddOnResponse, err := gql.CreateAddOn(ctx, client, input)

		if err != nil {
			return err
		}

		dsn := createAddOnResponse.CreateAddOn.AddOn.Token

		fmt.Fprintln(io.Out, "A Sentry project was created. Now setting the SENTRY_DSN secret and deploying.")
		secrets.SetSecretsAndDeploy(ctx, gql.ToAppCompact(targetApp), map[string]string{
			"SENTRY_DSN": dsn,
		}, false, false)

		return nil
	} else {
		fmt.Fprintln(io.Out, "A Sentry project already exists for this app")
	}

	return
}
