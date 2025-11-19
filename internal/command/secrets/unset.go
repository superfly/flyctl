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
		long  = `Unset one or more encrypted secrets for an application`
		short = long
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
	ctx, flapsClient, app, err := flapsutil.SetClient(ctx, nil, appName)
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

	return DeploySecrets(ctx, app, args)
}
