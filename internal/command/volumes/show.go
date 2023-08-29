package volumes

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
)

func newShow() (cmd *cobra.Command) {
	const (
		long = `Show details of an app's volume. Requires the volume's ID
number to operate. This can be found through the volumes list command`

		short = "Show details of an app's volume"
	)

	cmd = command.New("show <id>", short, long, runShow,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)
	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.JSONOutput(),
		flag.App(),
		flag.AppConfig(),
	)

	return
}

func runShow(ctx context.Context) error {
	cfg := config.FromContext(ctx)
	client := client.FromContext(ctx).API()

	volumeID := flag.FirstArg(ctx)

	appName := appconfig.NameFromContext(ctx)
	if volumeID == "" && appName == "" {
		return fmt.Errorf("volume ID or app required")
	}

	if appName == "" {
		n, err := client.GetAppNameFromVolume(ctx, volumeID)
		if err != nil {
			return err
		}
		appName = *n
	}

	flapsClient, err := flaps.NewFromAppName(ctx, appName)
	if err != nil {
		return err
	}

	var volume *api.Volume
	if volumeID == "" {
		volumes, err := flapsClient.GetVolumes(ctx)
		if err != nil {
			return err
		}
		app, err := client.GetApp(ctx, appName)
		if err != nil {
			return err
		}
		volume, err = selectVolume(ctx, volumes, app)
		if err != nil {
			return err
		}
	} else {
		volume, err = flapsClient.GetVolume(ctx, volumeID)
		if err != nil {
			return fmt.Errorf("failed retrieving volume: %w", err)
		}
	}

	out := iostreams.FromContext(ctx).Out

	if cfg.JSONOutput {
		return render.JSON(out, volume)
	}

	return printVolume(out, volume, appName)
}
