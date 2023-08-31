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
	"github.com/superfly/flyctl/internal/future"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
)

func newCreate() *cobra.Command {
	const (
		long = `Create new volume for app. --region flag must be included to specify
region the volume exists in. --size flag is optional, defaults to 3,
sets the size as the number of gigabytes the volume will consume.`

		short = "Create new volume for app"

		usage = "create <volumename>"
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
			Default:     3,
			Description: "Size of volume in gigabytes",
		},
		flag.Bool{
			Name:        "no-encryption",
			Description: "Do not encrypt the volume contents",
			Default:     false,
		},
		flag.Bool{
			Name:        "require-unique-zone",
			Description: "Require volume to be placed in separate hardware zone from existing volumes",
			Default:     true,
		},
		flag.String{
			Name:        "snapshot-id",
			Description: "Create volume from a specified snapshot",
		},
		flag.String{
			Name: "host-dedication-id",
		},
		flag.Yes(),
		flag.Int{
			Name:        "count",
			Shorthand:   "n",
			Default:     1,
			Description: "Number of volumes to create",
		},
	)

	flag.Add(cmd, flag.JSONOutput())
	return cmd
}

func runCreate(ctx context.Context) error {
	var (
		cfg    = config.FromContext(ctx)
		client = client.FromContext(ctx).API()

		volumeName = flag.FirstArg(ctx)
		appName    = appconfig.NameFromContext(ctx)
		count      = flag.GetInt(ctx, "count")
	)

	flapsClient, err := flaps.NewFromAppName(ctx, appName)
	if err != nil {
		return err
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	// pre-fetch platform regions from API in background
	prompt.PlatformRegions(ctx)

	// fetch AppBasic in the background while we prompt for confirmation
	appFuture := future.Spawn(func() (*api.AppBasic, error) {
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

	var region *api.Region
	if region, err = prompt.Region(ctx, !app.Organization.PaidPlan, prompt.RegionParams{
		Message: "",
	}); err != nil {
		return err
	}

	var snapshotID *string
	if flag.GetString(ctx, "snapshot-id") != "" {
		snapshotID = api.StringPointer(flag.GetString(ctx, "snapshot-id"))
	}

	input := api.CreateVolumeRequest{
		Name:              volumeName,
		Region:            region.Code,
		SizeGb:            api.Pointer(flag.GetInt(ctx, "size")),
		Encrypted:         api.Pointer(!flag.GetBool(ctx, "no-encryption")),
		RequireUniqueZone: api.Pointer(flag.GetBool(ctx, "require-unique-zone")),
		SnapshotID:        snapshotID,
		HostDedicationId:  flag.GetString(ctx, "host-dedication-id"),
	}
	out := iostreams.FromContext(ctx).Out
	for i := 0; i < count; i++ {
		volume, err := flapsClient.CreateVolume(ctx, input)
		if err != nil {
			return fmt.Errorf("failed creating volume: %w", err)
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
	var volumeName = flag.FirstArg(ctx)

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

	const msg = "Warning! Individual volumes are pinned to individual hosts. You should create two or more volumes per application. You will have downtime if you only create one. Learn more at https://fly.io/docs/reference/volumes/"
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
