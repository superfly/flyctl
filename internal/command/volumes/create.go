package volumes

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/future"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newCreate() *cobra.Command {
	const (
		short = "Create a new volume for an app."
		long  = "Create a new volume for an app. Volumes are persistent storage for Fly Machines. Learn how to add a volume to your app: https://fly.io/docs/launch/volume-storage/."
		usage = "create <volume name>"
	)

	cmd := command.New(usage, short, long, runCreate,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Region(),
		flag.Int{
			Name:        "size",
			Shorthand:   "s",
			Default:     deploy.DefaultVolumeInitialSizeGB,
			Description: "The size of volume in gigabytes",
		},
		flag.Int{
			Name:        "snapshot-retention",
			Default:     5,
			Description: "Snapshot retention in days",
		},
		flag.Bool{
			Name:        "no-encryption",
			Description: "Do not encrypt the volume contents. Volume contents are encrypted by default.",
			Default:     false,
		},
		flag.Bool{
			Name:        "require-unique-zone",
			Description: "Place the volume in a separate hardware zone from existing volumes to help ensure availability",
			Default:     true,
		},
		flag.String{
			Name:        "snapshot-id",
			Description: "Create the volume from the specified snapshot",
		},
		flag.String{
			Name:        "fs-type",
			Description: "Filesystem of this volume. It must be either ext4 or raw. Default is ext4.",
			Hidden:      true,
			Default:     "ext4",
		},
		flag.Yes(),
		flag.Int{
			Name:        "count",
			Shorthand:   "n",
			Default:     1,
			Description: "The number of volumes to create",
		},
		flag.VMSizeFlags,
	)

	flag.Add(cmd, flag.JSONOutput())
	return cmd
}

func runCreate(ctx context.Context) error {
	var (
		cfg    = config.FromContext(ctx)
		client = flyutil.ClientFromContext(ctx)

		volumeName = flag.FirstArg(ctx)
		appName    = appconfig.NameFromContext(ctx)
		count      = flag.GetInt(ctx, "count")
	)

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return err
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	// pre-fetch platform regions from API in background
	prompt.PlatformRegions(ctx)

	// fetch AppBasic in the background while we prompt for confirmation
	appFuture := future.Spawn(func() (*fly.AppBasic, error) {
		return client.GetAppBasic(ctx, appName)
	})

	if confirm, err := confirmVolumeCreate(ctx, appName); err != nil {
		return err
	} else if !confirm {
		return nil
	}

	app, err := appFuture.Get()
	if err != nil {
		return err
	}

	var region *fly.Region
	if region, err = prompt.Region(ctx, !app.Organization.PaidPlan, prompt.RegionParams{
		Message: "",
	}); err != nil {
		return err
	}

	var snapshotID *string
	if flag.GetString(ctx, "snapshot-id") != "" {
		snapshotID = fly.StringPointer(flag.GetString(ctx, "snapshot-id"))
	}

	var fsType *string
	if flag.IsSpecified(ctx, "fs-type") {
		s := flag.GetString(ctx, "fs-type")
		if s != "ext4" && s != "raw" {
			return fmt.Errorf(`filesystem %q must be either "ext4" or "raw"`, s)
		}
		fsType = &s
	}

	computeRequirements, err := flag.GetMachineGuest(ctx, nil)
	if err != nil {
		return err
	}

	input := fly.CreateVolumeRequest{
		Name:                volumeName,
		Region:              region.Code,
		SizeGb:              fly.Pointer(flag.GetInt(ctx, "size")),
		Encrypted:           fly.Pointer(!flag.GetBool(ctx, "no-encryption")),
		RequireUniqueZone:   fly.Pointer(flag.GetBool(ctx, "require-unique-zone")),
		SnapshotID:          snapshotID,
		ComputeRequirements: computeRequirements,
		SnapshotRetention:   fly.Pointer(flag.GetInt(ctx, "snapshot-retention")),
		FSType:              fsType,
	}
	out := iostreams.FromContext(ctx).Out
	for i := 0; i < count; i++ {
		volume, err := flapsClient.CreateVolume(ctx, input)
		if err != nil {
			return err
		}

		if cfg.JSONOutput {
			err = render.JSON(out, volume)
		} else {
			err = printVolume(out, volume, appName)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func confirmVolumeCreate(ctx context.Context, appName string) (bool, error) {
	volumeName := flag.FirstArg(ctx)

	// If the Yes flag has been supplied we skip the warning entirely, so no
	// need to query for the set of volumes
	if flag.GetYes(ctx) {
		return true, nil
	}

	if flag.GetInt(ctx, "count") > 1 {
		return true, nil
	}

	// If we have more than 0 volumes with this name already, return early
	if matches, err := countVolumesMatchingName(ctx, volumeName); err != nil {
		return false, err
	} else if matches > 0 {
		return true, nil
	}

	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	const msg = "Warning! Every volume is pinned to a specific physical host. You should create two or more volumes per application to avoid downtime. Learn more at https://fly.io/docs/volumes/overview/"
	fmt.Fprintln(io.ErrOut, colorize.Red(msg))

	switch confirmed, err := prompt.Confirm(ctx, "Do you still want to use the volumes feature?"); {
	case err == nil:
		return confirmed, nil
	case prompt.IsNonInteractive(err):
		return false, prompt.NonInteractiveError("yes flag must be specified when not running interactively")
	default:
		return false, err
	}
}
