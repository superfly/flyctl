package image

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
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

	switch app.ImageDetails.Registry {

	case "unknown":
		rows := make([][]string, 0, len(app.Machines.Nodes))

		for _, machine := range app.Machines.Nodes {
			rows = append(rows, []string{
				machine.ID,
				machine.Name,
				machine.Config.Image,
			})
		}

		return render.Table(io.Out, "Machines", rows, "ID", "Name", "Image")
	default:

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

		return render.VerticalTable(io.Out, "Image", obj,
			"Registry",
			"Repository",
			"Tag",
			"Version",
			"Digest",
		)
	}
}
