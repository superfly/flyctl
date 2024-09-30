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

func newFork() *cobra.Command {
	const (
		short = "Fork the specified volume."

		long = short + ` Volume forking creates an independent copy of a storage volume for backup, testing, and experimentation without altering the original data.`

		usage = "fork <volume id>"
	)

	cmd := command.New(usage, short, long, runFork,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of the new volume",
		},
		flag.Bool{
			Name:        "require-unique-zone",
			Description: "Place the volume in a separate hardware zone from existing volumes. This is the default.",
			Default:     true,
		},
		flag.String{
			Name:        "region",
			Shorthand:   "r",
			Description: "The target region. By default, the new volume will be created in the source volume's region.",
		},
		flag.VMSizeFlags,
	)

	flag.Add(cmd, flag.JSONOutput())
	return cmd
}

func runFork(ctx context.Context) error {
	var (
		cfg     = config.FromContext(ctx)
		appName = appconfig.NameFromContext(ctx)
		volID   = flag.FirstArg(ctx)
		client  = flyutil.ClientFromContext(ctx)
	)

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return err
	}

	var vol *fly.Volume
	if volID == "" {
		app, err := client.GetAppBasic(ctx, appName)
		if err != nil {
			return err
		}
		vol, err = selectVolume(ctx, flapsClient, app)
		if err != nil {
			return err
		}
	} else {
		vol, err = flapsClient.GetVolume(ctx, volID)
		if err != nil {
			return fmt.Errorf("failed to get volume: %w", err)
		}
	}

	name := vol.Name
	if flag.IsSpecified(ctx, "name") {
		name = flag.GetString(ctx, "name")
	}

	region := flag.GetString(ctx, "region")

	var attachedMachineImage string
	var attachedMachineGuest *fly.MachineGuest
	if vol.AttachedMachine != nil {
		m, err := flapsClient.Get(ctx, *vol.AttachedMachine)
		if err != nil {
			return err
		}
		attachedMachineGuest = m.Config.Guest
		attachedMachineImage = m.FullImageRef()
	}

	computeRequirements, err := flag.GetMachineGuest(ctx, attachedMachineGuest)
	if err != nil {
		return err
	}

	input := fly.CreateVolumeRequest{
		Name:                name,
		RequireUniqueZone:   fly.Pointer(flag.GetBool(ctx, "require-unique-zone")),
		SourceVolumeID:      &vol.ID,
		ComputeRequirements: computeRequirements,
		ComputeImage:        attachedMachineImage,
		Region:              region,
	}

	volume, err := flapsClient.CreateVolume(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to fork volume: %w", err)
	}

	out := iostreams.FromContext(ctx).Out

	if cfg.JSONOutput {
		return render.JSON(out, volume)
	}

	if err := printVolume(out, volume, appName); err != nil {
		return err
	}

	return nil
}
