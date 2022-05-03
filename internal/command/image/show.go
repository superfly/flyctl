package image

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
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

func runShow(ctx context.Context) (err error) {
	var (
		client  = client.FromContext(ctx).API()
		io      = iostreams.FromContext(ctx)
		appName = app.NameFromContext(ctx)
	)

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	var status *api.AppStatus

	if status, err = client.GetAppStatus(ctx, appName, true); err != nil {
		err = fmt.Errorf("failed retrieving app %s: %w", appName, err)

		return
	}

	if !status.Deployed && app.PlatformVersion == "" {
		_, err = fmt.Fprintln(io.Out, "App has not been deployed yet.")

		return
	}

	switch app.PlatformVersion {
	case "nomad":
		return showNomadImage(ctx, app)
	case "machines":
		return showMachineImage(ctx, app)
	}

	return nil
}

func showNomadImage(ctx context.Context, machine *api.App) error {
	var (
		client   = client.FromContext(ctx).API()
		cfg      = config.FromContext(ctx)
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		appName  = app.NameFromContext(ctx)
	)

	info, err := client.GetImageInfo(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed to get image info: %w", err)
	}

	if cfg.JSONOutput {
		return render.JSON(io.Out, info.ImageDetails)
	}

	if info.ImageVersionTrackingEnabled && info.ImageUpgradeAvailable {
		current := fmt.Sprintf("%s:%s", info.ImageDetails.Repository, info.ImageDetails.Tag)
		latest := fmt.Sprintf("%s:%s", info.LatestImageDetails.Repository, info.LatestImageDetails.Tag)

		if info.ImageDetails.Version != "" {
			current = fmt.Sprintf("%s %s", current, info.ImageDetails.Version)
		}

		if info.LatestImageDetails.Version != "" {
			latest = fmt.Sprintf("%s %s", latest, info.LatestImageDetails.Version)
		}

		var message = fmt.Sprintf("Update available! (%s -> %s)\n", current, latest)
		message += "Run `flyctl image update` to migrate to the latest image version.\n"

		fmt.Fprintln(io.ErrOut, colorize.Yellow(message))
	}

	image := info.ImageDetails

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

func showMachineImage(ctx context.Context, app *api.App) error {
	var (
		client = client.FromContext(ctx).API()
		io     = iostreams.FromContext(ctx)
	)

	// get machines
	machines, err := client.ListMachines(ctx, app.Name, "")
	if err != nil {
		return fmt.Errorf("failed to get machines: %w", err)
	}

	rows := [][]string{}

	for _, machine := range machines {
		var image = machine.Config.ImageRef

		var version = "N/A"

		if image.Labels != nil && image.Labels["version"] != "" {
			version = image.Labels["version"]
		}

		rows = append(rows, []string{
			machine.ID,
			image.Registry,
			image.Repository,
			image.Tag,
			version,
			image.Digest,
		})
	}

	return render.Table(
		io.Out,
		"Machine Image Details",
		rows,
		"Machine ID",
		"Registry",
		"Repository",
		"Tag",
		"Version",
		"Digest",
	)

}
