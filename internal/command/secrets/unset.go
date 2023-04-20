package secrets

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
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
	client := client.FromContext(ctx).API()
	appName := appconfig.NameFromContext(ctx)
	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	return UnsetSecretsAndDeploy(ctx, app, flag.Args(ctx), flag.GetBool(ctx, "stage"), flag.GetBool(ctx, "detach"))
}

func UnsetSecretsAndDeploy(ctx context.Context, app *api.AppCompact, secrets []string, stage bool, detach bool) error {
	client := client.FromContext(ctx).API()
	release, err := client.UnsetSecrets(ctx, app.Name, secrets)
	if err != nil {
		return err
	}

	return deployForSecrets(ctx, app, release, stage, detach)
}
