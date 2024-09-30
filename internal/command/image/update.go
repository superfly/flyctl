package image

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
)

func newUpdate() *cobra.Command {
	const (
		long = `Update the app's image to the latest available version.
The update will perform a rolling restart against each Machine, which may result in a brief service disruption.`
		short = "Updates the app's image to the latest available version."
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
			Name:        "image",
			Description: "Target a specific image",
		},
		flag.Bool{
			Name:        "skip-health-checks",
			Description: "Skip waiting for health checks inbetween VM updates.",
			Default:     false,
		},
	)

	return cmd
}

func runUpdate(ctx context.Context) error {
	var (
		appName = appconfig.NameFromContext(ctx)
		client  = flyutil.ClientFromContext(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	ctx, err = apps.BuildContext(ctx, app)
	if err != nil {
		return err
	}

	if app.IsPostgresApp() {
		return updatePostgresOnMachines(ctx, app)
	}
	return updateImageForMachines(ctx, app)
}
