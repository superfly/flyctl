package image

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
)

func newUpdate() *cobra.Command {
	const (
		long = `This will update the application's image to the latest available version.
The update will perform a rolling restart against each VM, which may result in a brief service disruption.`
		short = "Updates the app's image to the latest available version. (Fly Postgres only)"
		usage = "update"
	)

	cmd := command.New(usage, short, long, runUpdate,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Yes(),
		flag.String{
			Name:        "strategy",
			Description: "Deployment strategy. (Nomad only)",
		},
		flag.Bool{
			Name:        "detach",
			Description: "Return immediately instead of monitoring update progress. (Nomad only)",
		},
		flag.String{
			Name:        "image",
			Description: "Target a specific image. (Machines only)",
		},
		flag.Bool{
			Name:        "skip-health-checks",
			Description: "Skip waiting for health checks inbetween VM updates. (Machines only)",
			Default:     false,
		},
	)

	return cmd
}

func runUpdate(ctx context.Context) error {
	var (
		appName = app.NameFromContext(ctx)
		client  = client.FromContext(ctx).API()
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	ctx, err = apps.BuildContext(ctx, app)
	if err != nil {
		return err
	}

	switch app.PlatformVersion {
	case "nomad":
		return updateImageForNomad(ctx)
	case "machines":
		if app.IsPostgresApp() {
			return updatePostgresOnMachines(ctx, app)
		}
		return updateImageForMachines(ctx, app)
	default:
		return fmt.Errorf("unable to determine platform version. please contact support")
	}
}
