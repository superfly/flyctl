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
		long  = `Deploy staged secrets for an application`
		short = long
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
	ctx, flapsClient, app, err := flapsutil.SetClient(ctx, nil, appName)
	if err != nil {
		return err
	}

	machines, _, err := flapsClient.ListFlyAppsMachines(ctx)
	if err != nil {
		return err
	}

	if !(app.Deployed && len(machines) > 0) {
		return flyerr.GenericErr{
			Err:      "no machines available to deploy",
			Descript: "'fly secrets deploy' will only work if the app has been deployed and there are machines available",
			Suggest:  "Try 'fly deploy' first",
		}
	}

	return DeploySecrets(ctx, app, DeploymentArgs{
		Stage:    false,
		Detach:   flag.GetBool(ctx, "detach"),
		CheckDNS: flag.GetBool(ctx, "dns-checks"),
	})
}
