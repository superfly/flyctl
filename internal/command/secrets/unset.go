package secrets

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
)

func newUnset() (cmd *cobra.Command) {
	const (
		long = `Remove one or more secrets from an app, by name.

After removing the secrets, the command updates all of the app's existing
Machines with the currently deployed configuration and image (not your
local fly.toml) so the variables are gone, which restarts them. With
` + "`--detach`" + `, it doesn't wait for health checks.

With ` + "`--stage`" + `, the secrets are removed but no Machines are updated;
running Machines keep the old values until you run ` + "`fly secrets deploy`" + `
or ` + "`fly deploy`" + `.`
		short = "Remove one or more secrets from an app"
		usage = "unset [flags] NAME NAME ..."
	)

	cmd = command.New(usage, short, long, runUnset, command.RequireSession, command.RequireAppName)

	flag.Add(cmd,
		sharedFlags,
	)

	cmd.Args = cobra.MinimumNArgs(1)

	return cmd
}

func runUnset(ctx context.Context) (err error) {
	appName := appconfig.NameFromContext(ctx)

	flapsClient := flapsutil.ClientFromContext(ctx)

	app, err := flapsClient.GetApp(ctx, appName)
	if err != nil {
		return err
	}

	return UnsetSecretsAndDeploy(ctx, flapsClient, app, flag.Args(ctx), DeploymentArgs{
		Stage:    flag.GetBool(ctx, "stage"),
		Detach:   flag.GetBool(ctx, "detach"),
		CheckDNS: flag.GetBool(ctx, "dns-checks"),
	})
}

func UnsetSecretsAndDeploy(ctx context.Context, flapsClient flapsutil.FlapsClient, app *flaps.App, secrets []string, args DeploymentArgs) error {
	if err := appsecrets.Update(ctx, flapsClient, app.Name, nil, secrets); err != nil {
		return fmt.Errorf("update secrets: %w", err)
	}

	return DeploySecrets(ctx, app.Name, args)
}
