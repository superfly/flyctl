package secrets

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
)

func newUnsetAll() (cmd *cobra.Command) {
	const (
		long  = `Unset all encrypted secrets for an application`
		short = long
		usage = "unset-all"
	)

	cmd = command.New(usage, short, long, runUnsetAll, command.RequireSession, command.RequireAppName)

	flag.Add(cmd,
		sharedFlags,
	)

	return cmd
}

func runUnsetAll(ctx context.Context) (err error) {
	client := flyutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	app, err := client.GetAppCompact(ctx, appName)

	if err != nil {
		return err
	}

	appSecrets, err := client.GetAppSecrets(ctx, appName)

	if err != nil {
		return err
	}

	var secrets []string
	for _, secret := range appSecrets {
		secrets = append(secrets, secret.Name)
	}

	return UnsetSecretsAndDeploy(ctx, app, secrets, flag.GetBool(ctx, "stage"), flag.GetBool(ctx, "detach"))
}
