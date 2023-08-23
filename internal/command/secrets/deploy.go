package secrets

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyerr"
)

func newDeploy() (cmd *cobra.Command) {
	const (
		long  = `Deploy staged secrets for an application`
		short = long
		usage = "deploy [flags]"
	)

	cmd = command.New(usage, short, long, runDeploy, command.RequireSession, command.RequireAppName)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Detach(),
	)

	return cmd
}

func runDeploy(ctx context.Context) (err error) {
	client := client.FromContext(ctx).API()
	appName := appconfig.NameFromContext(ctx)
	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("could not create flaps client: %w", err)
	}

	machines, _, err := flapsClient.ListFlyAppsMachines(ctx)
	if err != nil {
		return err
	}

	if app.PlatformVersion != appconfig.MachinesPlatform {
		return flyerr.GenericErr{
				Err:      "secrets deploy is only supported for machine apps",
		}
	}

	if !(app.Deployed && len(machines) > 0) {
		return flyerr.GenericErr{
			Err:      "no machines available to deploy",
			Descript: "'fly secrets deploy' will only work if the app has been deployed and there are machines available",
			Suggest:  "Try 'fly deploy' first",
		}
	}

	return DeploySecrets(ctx, app, false, flag.GetBool(ctx, "detach"))
}
