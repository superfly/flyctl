package image

import (
	"context"
	"fmt"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newShow() *cobra.Command {
	const (
		long  = "Show image details."
		short = "Show image details."

		usage = "show"
	)

	cmd := command.New(usage, short, long, runShow,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runShow(ctx context.Context) error {
	var (
		client  = client.FromContext(ctx).API()
		cfg     = config.FromContext(ctx)
		io      = iostreams.FromContext(ctx)
		appName = app.NameFromContext(ctx)
	)

	app, err := client.GetImageInfo(ctx, appName)
	if err != nil {
		return err
	}

	if cfg.JSONOutput {
		return render.JSON(io.Out, app.ImageDetails)
	}

	if app.ImageVersionTrackingEnabled && app.ImageUpgradeAvailable {
		current := fmt.Sprintf("%s:%s", app.ImageDetails.Repository, app.ImageDetails.Tag)
		latest := fmt.Sprintf("%s:%s", app.LatestImageDetails.Repository, app.LatestImageDetails.Tag)

		if app.ImageDetails.Version != "" {
			current = fmt.Sprintf("%s %s", current, app.ImageDetails.Version)
		}

		if app.LatestImageDetails.Version != "" {
			latest = fmt.Sprintf("%s %s", latest, app.LatestImageDetails.Version)
		}

		fmt.Fprintln(io.ErrOut, aurora.Yellow(fmt.Sprintf("Update available! (%s -> %s)", current, latest)))
		fmt.Fprintln(io.ErrOut, aurora.Yellow("Run `fly image update` to migrate to the latest image version.\n"))
	}

	image := app.ImageDetails

	if image.Version == "" {
		image.Version = "N/A"
	}

	obj := [][]string{
		{
			image.Registry,
			image.Repository,
			image.Tag,
			image.Version,
			image.Digest,
		},
	}

	return render.VerticalTable(io.Out, "Deployment Status", obj,
		"Registry",
		"Repository",
		"Tag",
		"Version",
		"Digest",
	)
}
