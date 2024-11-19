package secrets

import (
	"context"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
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
	client := flyutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	return UnsetSecretsAndDeploy(ctx, app, flag.Args(ctx), flag.GetBool(ctx, "stage"), flag.GetBool(ctx, "detach"))
}

func UnsetSecretsAndDeploy(ctx context.Context, app *fly.AppCompact, secrets []string, stage bool, detach bool) error {
	client := flyutil.ClientFromContext(ctx)
	if _, err := client.UnsetSecrets(ctx, app.Name, secrets); err != nil {
		return err
	}

	return DeploySecrets(ctx, app, stage, detach)
}
