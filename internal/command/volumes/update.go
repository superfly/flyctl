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

func newUpdate() *cobra.Command {
	const (
		short = "Update a volume for an app."

		long = short + ` Volumes are persistent storage for
		Fly Machines. Learn how to add a volume to
		your app: https://fly.io/docs/apps/volume-storage/`

		usage = "update <volumename>"
	)

	cmd := command.New(usage, short, long, runUpdate,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Int{
			Name:        "snapshot-retention",
			Description: "Snapshot retention in days (min 5)",
		},
	)

	flag.Add(cmd, flag.JSONOutput())
	return cmd
}

func runUpdate(ctx context.Context) error {
	var (
		cfg      = config.FromContext(ctx)
		client   = client.FromContext(ctx).API()
		volumeID = flag.FirstArg(ctx)
	)

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

	var snapshotRetention *int
	if flag.GetInt(ctx, "snapshot-retention") != 0 {
		snapshotRetention = api.Pointer(flag.GetInt(ctx, "snapshot-retention"))
	}

	out := iostreams.FromContext(ctx).Out
	input := api.UpdateVolumeRequest{
		SnapshotRetention: snapshotRetention,
	}

	updatedVolume, err := flapsClient.UpdateVolume(ctx, volumeID, input)
	if err != nil {
		return fmt.Errorf("failed updating volume: %w", err)
	}

	if cfg.JSONOutput {
		return render.JSON(out, updatedVolume)
	}

	return printVolume(out, updatedVolume, appName)
}
