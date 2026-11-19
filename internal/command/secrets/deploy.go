package secrets

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyerr"
)

func newDeploy() (cmd *cobra.Command) {
	const (
		long = `Apply staged secret changes, such as those made with ` + "`--stage`" + `, to
an app's Machines. The command updates all of the app's existing Machines
with the currently deployed configuration and image (not your local
fly.toml), which restarts them. With ` + "`--detach`" + `, it doesn't wait for
health checks.

The app must already be deployed and have Machines; otherwise the command
fails and you should run ` + "`fly deploy`" + ` instead.`
		short = "Deploy staged secrets to an app's Machines"
		usage = "deploy [flags]"
	)

	cmd = command.New(usage, short, long, runDeploy, command.RequireSession, command.RequireAppName)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Detach(),
		flag.Bool{
			Name:        "dns-checks",
			Description: "Perform DNS checks during deployment",
			Default:     true,
		},
	)

	return cmd
}

func runDeploy(ctx context.Context) (err error) {
	appName := appconfig.NameFromContext(ctx)

	flapsClient := flapsutil.ClientFromContext(ctx)

	app, err := flapsClient.GetApp(ctx, appName)
	if err != nil {
		return err
	}

	machines, _, err := flapsClient.ListFlyAppsMachines(ctx, appName)
	if err != nil {
		return err
	}

	if !app.Deployed() || len(machines) <= 0 {
		return flyerr.GenericErr{
			Err:      "no machines available to deploy",
			Descript: "'fly secrets deploy' will only work if the app has been deployed and there are machines available",
			Suggest:  "Try 'fly deploy' first",
		}
	}

	return DeploySecrets(ctx, app.Name, DeploymentArgs{
		Stage:    false,
		Detach:   flag.GetBool(ctx, "detach"),
		CheckDNS: flag.GetBool(ctx, "dns-checks"),
	})
}
