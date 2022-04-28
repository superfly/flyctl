package image

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
)

func newShow() *cobra.Command {
	const (
		long  = "Show image details."
		short = long + "\n"

		usage = "show"
	)

	cmd := command.New(usage, short, long, runShow,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runShow(ctx context.Context) error {
	var (
		client   = client.FromContext(ctx).API()
		cfg      = config.FromContext(ctx)
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		appName  = app.NameFromContext(ctx)
	)

	app, err := client.GetImageInfo(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed to get image info: %w", err)
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

		var message = fmt.Sprintf("Update available! (%s -> %s)\n", current, latest)
		message += "Run `flyctl image update` to migrate to the latest image version.\n"

		fmt.Fprintln(io.ErrOut, colorize.Yellow(message))
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
