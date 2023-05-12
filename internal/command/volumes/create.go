package volumes

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
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
		flag.Yes(),
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
	)

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

	input := api.CreateVolumeInput{
		AppID:             app.ID,
		Name:              volumeName,
		Region:            region.Code,
		SizeGb:            flag.GetInt(ctx, "size"),
		Encrypted:         !flag.GetBool(ctx, "no-encryption"),
		RequireUniqueZone: flag.GetBool(ctx, "require-unique-zone"),
		SnapshotID:        snapshotID,
	}

	volume, err := client.CreateVolume(ctx, input)
	if err != nil {
		return fmt.Errorf("failed creating volume: %w", err)
	}

	out := iostreams.FromContext(ctx).Out

	if cfg.JSONOutput {
		return render.JSON(out, volume)
	}

	return printVolume(out, volume)
}

func confirmVolumeCreate(ctx context.Context, appName string) (bool, error) {
	var volumeName = flag.FirstArg(ctx)

	// If the Yes flag has been supplied we skip the warning entirely, so no
	// need to query for the set of volumes
	if flag.GetYes(ctx) {
		return true, nil
	}

	// If we have more than 0 volues with this name already, return early
	if matches, err := countVolumesMatchingName(ctx, appName, volumeName); err != nil {
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
