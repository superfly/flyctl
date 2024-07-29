package volumes

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newShow() (cmd *cobra.Command) {
	const (
		short = "Show the details of the specified volume."

		long = short
	)

	cmd = command.New("show <volume id>", short, long, runShow,
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
	client := flyutil.ClientFromContext(ctx)

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

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return err
	}

	var volume *fly.Volume
	if volumeID == "" {
		app, err := client.GetAppBasic(ctx, appName)
		if err != nil {
			return err
		}
		volume, err = selectVolume(ctx, flapsClient, app)
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
