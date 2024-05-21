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

func newUpdate() *cobra.Command {
	const (
		short = "Update a volume for an app."

		long = short + ` Volumes are persistent storage for
		Fly Machines.`

		usage = "update <volume id>"
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
			Description: "Snapshot retention in days",
		},
		flag.Bool{
			Name:        "scheduled-snapshots",
			Description: "Activate/deactivate scheduled automatic snapshots",
		},
	)

	flag.Add(cmd, flag.JSONOutput())
	return cmd
}

func runUpdate(ctx context.Context) error {
	var (
		cfg      = config.FromContext(ctx)
		client   = flyutil.ClientFromContext(ctx)
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

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return err
	}

	var snapshotRetention *int
	if flag.GetInt(ctx, "snapshot-retention") != 0 {
		snapshotRetention = fly.Pointer(flag.GetInt(ctx, "snapshot-retention"))
	}

	out := iostreams.FromContext(ctx).Out
	input := fly.UpdateVolumeRequest{
		SnapshotRetention: snapshotRetention,
	}

	if flag.IsSpecified(ctx, "scheduled-snapshots") {
		input.AutoBackupEnabled = fly.BoolPointer(flag.GetBool(ctx, "scheduled-snapshots"))
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
