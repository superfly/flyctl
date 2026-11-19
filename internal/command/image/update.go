package image

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
)

func newUpdate() *cobra.Command {
	const (
		long = `Update each of the app's Machines to the newest version of the image it
runs, or to the image given with --image. Each Machine restarts as it's
updated, which can briefly interrupt service. Only images whose versions
Fly.io tracks, such as Fly Postgres, have newer versions to move to.

You see each Machine's config change and confirm it; pass --yes to skip the
prompts, and without a terminal --yes is required. For Postgres apps,
Machines not running the official Postgres image are skipped, you confirm
once, and replicas are updated before the primary.`
		short = "Update an app's Machines to the latest image version"
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
			Description: "Skip waiting for health checks between VM updates.",
			Default:     false,
		},
	)

	return cmd
}

func runUpdate(ctx context.Context) error {
	var (
		appName = appconfig.NameFromContext(ctx)
	)

	app, err := flapsutil.ClientFromContext(ctx).GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	ctx, err = apps.BuildContextForApp(ctx, app)
	if err != nil {
		return err
	}

	if flapsutil.IsPostgresApp(app) {
		return updatePostgresOnMachines(ctx, app)
	}

	return updateImageForMachines(ctx, app)
}
