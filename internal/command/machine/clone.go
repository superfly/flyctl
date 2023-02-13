package machine

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"
)

func newClone() *cobra.Command {
	const (
		short = "Clone a Fly machine"
		long  = short + "\n"

		usage = "clone <id>"
	)

	cmd := command.New(usage, short, long, runMachineClone,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "region",
			Description: "Target region for the new machine",
		},
		flag.String{
			Name:        "name",
			Description: "Optional name for the new machine",
		},
		flag.String{
			Name:        "from-snapshot",
			Description: "Clone attached volumes and restore from snapshot, use 'last' for most recent snapshot. The default is an empty volume",
		},
		flag.String{
			Name:        "attach-volume",
			Description: "Existing volume to attach to the new machine",
		},
	)

	return cmd
}

func runMachineClone(ctx context.Context) (err error) {
	var (
		machineID = flag.FirstArg(ctx)
		out       = iostreams.FromContext(ctx).Out
		appName   = app.NameFromContext(ctx)
		io        = iostreams.FromContext(ctx)
		colorize  = io.ColorScheme()
		client    = client.FromContext(ctx).API()
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		help := newClone().Help()

		if help != nil {
			fmt.Println(help)

		}

		fmt.Println()

		return err
	}

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("could not make flaps client: %w", err)
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	source, err := flapsClient.Get(ctx, machineID)
	if err != nil {
		return err
	}

	region := flag.GetString(ctx, "region")
	if region == "" {
		region = source.Region
	}

	fmt.Fprintf(out, "Cloning machine %s into region %s\n", colorize.Bold(source.ID), colorize.Bold(region))

	targetConfig := source.Config

	image := fmt.Sprintf("%s/%s", source.ImageRef.Registry, source.ImageRef.Repository)
	tag := source.ImageRef.Tag
	digest := source.ImageRef.Digest

	if tag != "" && digest != "" {
		image = fmt.Sprintf("%s:%s@%s", image, tag, digest)
	} else if digest != "" {
		image = fmt.Sprintf("%s@%s", image, digest)
	} else if tag != "" {
		image = fmt.Sprintf("%s:%s", image, tag)
	}

	targetConfig.Image = image

	for _, mnt := range source.Config.Mounts {
		var vol *api.Volume

		if volID := flag.GetString(ctx, "attach-volume"); volID != "" {

			fmt.Fprintf(out, "Attaching existing volume %s\n", colorize.Bold(volID))

			vol, err = client.GetVolume(ctx, volID)
			if err != nil {
				return fmt.Errorf("could not get existing volume: %w", err)
			}

			if vol.IsAttached() {
				return fmt.Errorf("volume %s is already attached to a machine", vol.ID)
			}

		} else {
			var snapshotID *string
			switch snapID := flag.GetString(ctx, "from-snapshot"); snapID {
			case "last":
				snapshots, err := client.GetVolumeSnapshots(ctx, mnt.Volume)
				if err != nil {
					return err
				}
				if len(snapshots) > 0 {
					snapshot := lo.MaxBy(snapshots, func(i, j api.Snapshot) bool { return i.CreatedAt.After(j.CreatedAt) })
					snapshotID = &snapshot.ID
					fmt.Fprintf(out, "Creating new volume from snapshot %s of %s\n", colorize.Bold(*snapshotID), colorize.Bold(mnt.Volume))
				} else {
					fmt.Fprintf(out, "No snapshot for source volume %s, the new volume will start empty\n", colorize.Bold(mnt.Volume))
					snapshotID = nil
				}
			case "":
				fmt.Fprintf(out, "Volume '%s' will start empty\n", colorize.Bold(mnt.Name))
			default:
				snapshotID = &snapID
				fmt.Fprintf(io.Out, "Creating new volume from snapshot: %s", colorize.Bold(*snapshotID))
			}

			volInput := api.CreateVolumeInput{
				AppID:             app.ID,
				Name:              mnt.Name,
				Region:            region,
				SizeGb:            mnt.SizeGb,
				Encrypted:         mnt.Encrypted,
				SnapshotID:        snapshotID,
				RequireUniqueZone: false,
			}
			vol, err = client.CreateVolume(ctx, volInput)
			if err != nil {
				return err
			}
		}

		targetConfig.Mounts = []api.MachineMount{
			{
				Volume: vol.ID,
				Path:   mnt.Path,
			},
		}
	}

	input := api.LaunchMachineInput{
		AppID:  app.Name,
		Name:   flag.GetString(ctx, "name"),
		Region: region,
		Config: targetConfig,
	}

	fmt.Fprintf(out, "Provisioning a new machine with image %s...\n", source.Config.Image)

	launchedMachine, err := flapsClient.Launch(ctx, input)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "  Machine %s has been created...\n", colorize.Bold(launchedMachine.ID))
	fmt.Fprintf(out, "  Waiting for machine %s to start...\n", colorize.Bold(launchedMachine.ID))

	// wait for a machine to be started
	err = mach.WaitForStartOrStop(ctx, launchedMachine, "start", time.Minute*5)
	if err != nil {
		return err
	}

	if err = watch.MachinesChecks(ctx, []*api.Machine{launchedMachine}); err != nil {
		return fmt.Errorf("error while watching health checks: %w", err)
	}

	fmt.Fprintf(out, "Machine has been successfully cloned!\n")

	return
}
