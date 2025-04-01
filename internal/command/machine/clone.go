package machine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/shlex"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"
)

func newClone() *cobra.Command {
	const (
		short = "Clone a Fly Machine"
		long  = "Clone a Fly Machine. The new Machine will be a copy of the specified Machine. If the original Machine has a volume, then a new empty volume will be created and attached to the new Machine."

		usage = "clone [machine_id]"
	)

	cmd := command.New(usage, short, long, runMachineClone,
		command.RequireSession,
		command.RequireAppName,
		command.LoadAppConfigIfPresent,
	)

	cmd.Args = cobra.RangeArgs(0, 1)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		selectFlag,
		flag.Region(),
		flag.String{
			Name:        "name",
			Description: "Optional name for the new Machine",
		},
		flag.String{
			Name:        "from-snapshot",
			Description: "Clone attached volumes and restore from snapshot, use 'last' for most recent snapshot. The default is an empty volume.",
		},
		flag.String{
			Name:        "attach-volume",
			Description: "Existing volume to attach to the new Machine in the form of <volume_id>[:/path/inside/machine]",
		},
		flag.String{
			Name:        "override-cmd",
			Description: "Set CMD on the new Machine to this value",
		},
		flag.Bool{
			Name:        "clear-cmd",
			Description: "Set empty CMD on the new Machine so it uses default CMD for the image",
		},
		flag.Bool{
			Name:        "clear-auto-destroy",
			Description: "Disable auto destroy setting on the new Machine",
		},
		flag.StringSlice{
			Name:        "standby-for",
			Description: "Comma separated list of Machine IDs to watch for. You can use '--standby-for=source' to create a standby for the cloned Machine.",
		},
		flag.Bool{
			Name:        "volume-requires-unique-zone",
			Description: "Require volume to be placed in separate hardware zone from existing volumes. Default true.",
			Default:     true,
		},
		flag.Detach(),
		flag.VMSizeFlags,
	)

	return cmd
}

func runMachineClone(ctx context.Context) (err error) {
	var (
		out      = iostreams.FromContext(ctx).Out
		appName  = appconfig.NameFromContext(ctx)
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
	)

	machineID := flag.FirstArg(ctx)
	haveMachineID := len(flag.Args(ctx)) > 0
	source, ctx, err := selectOneMachine(ctx, appName, machineID, haveMachineID)
	if err != nil {
		return err
	}

	if source.HostStatus != fly.HostStatusOk {
		return fmt.Errorf("the machine is on an unreachable host, try again later")
	}

	flapsClient := flapsutil.ClientFromContext(ctx)

	var vol *fly.Volume
	if volumeInfo := flag.GetString(ctx, "attach-volume"); volumeInfo != "" {
		splitVolumeInfo := strings.Split(volumeInfo, ":")
		volID := splitVolumeInfo[0]

		vol, err = flapsClient.GetVolume(ctx, volID)
		if err != nil {
			return fmt.Errorf("could not get existing volume: %w", err)
		}
	}

	region := flag.GetString(ctx, "region")
	if vol != nil && region != "" {
		if vol.Region != region {
			return fmt.Errorf("specified region %s but volume is in region %s, use the same region as the volume", colorize.Bold(region), colorize.Bold(vol.Region))
		}
	} else if vol != nil && region == "" {
		region = vol.Region
	} else if region == "" {
		region = source.Region
	}

	fmt.Fprintf(out, "Cloning Machine %s into region %s\n", colorize.Bold(source.ID), colorize.Bold(region))

	targetConfig := helpers.Clone(source.Config)

	targetConfig.Guest, err = flag.GetMachineGuest(ctx, targetConfig.Guest)
	if err != nil {
		return err
	}

	targetConfig.Image = source.FullImageRef()

	if flag.GetBool(ctx, "clear-cmd") {
		targetConfig.Init.Cmd = make([]string, 0)
	} else if targetCmd := flag.GetString(ctx, "override-cmd"); targetCmd != "" {
		theCmd, err := shlex.Split(targetCmd)
		if err != nil {
			return fmt.Errorf("error splitting cmd: %w", err)
		}
		targetConfig.Init.Cmd = theCmd
	}
	if flag.GetBool(ctx, "clear-auto-destroy") {
		targetConfig.AutoDestroy = false
	}
	if targetConfig.AutoDestroy {
		fmt.Fprintf(io.Out, "Auto destroy enabled and will destroy Machine on exit. Use --clear-auto-destroy to remove this setting.\n")
	}

	var volID string
	if volumeInfo := flag.GetString(ctx, "attach-volume"); volumeInfo != "" {
		splitVolumeInfo := strings.Split(volumeInfo, ":")
		volID = splitVolumeInfo[0]

		if len(source.Config.Mounts) > 1 {
			return fmt.Errorf("Can't use --attach-volume for Machines with more than 1 volume.")
		} else if len(source.Config.Mounts) == 0 && len(splitVolumeInfo) != 2 {
			return fmt.Errorf("Please specify a mount path on '%s' using <volume_id>:/path/inside/machine", volumeInfo)
		}

		// in case user passed a mount path
		if len(splitVolumeInfo) == 2 {
			// patches the source config so the loop below attaches the volume on the passed mount path
			if len(source.Config.Mounts) == 0 {
				source.Config.Mounts = []fly.MachineMount{
					{
						Path: splitVolumeInfo[1],
					},
				}
			} else if len(source.Config.Mounts) == 1 {
				fmt.Fprintf(io.Out, "Info: --attach-volume is overriding previous mount point from `%s` to `%s`.\n", source.Config.Mounts[0].Path, splitVolumeInfo[1])
				source.Config.Mounts[0].Path = splitVolumeInfo[1]
			}
		}
	}

	for _, mnt := range source.Config.Mounts {
		var vol *fly.Volume
		if volID != "" {
			fmt.Fprintf(out, "Attaching existing volume %s\n", colorize.Bold(volID))
			vol, err = flapsClient.GetVolume(ctx, volID)
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
				snapshots, err := flapsClient.GetVolumeSnapshots(ctx, mnt.Volume)
				if err != nil {
					return err
				}
				if len(snapshots) > 0 {
					snapshot := lo.MaxBy(snapshots, func(i, j fly.VolumeSnapshot) bool { return i.CreatedAt.After(j.CreatedAt) })
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
				fmt.Fprintf(io.Out, "Creating new volume from snapshot: %s\n", colorize.Bold(*snapshotID))
			}

			volInput := fly.CreateVolumeRequest{
				Name:                mnt.Name,
				Region:              region,
				SizeGb:              &mnt.SizeGb,
				Encrypted:           &mnt.Encrypted,
				SnapshotID:          snapshotID,
				RequireUniqueZone:   fly.Pointer(flag.GetBool(ctx, "volume-requires-unique-zone")),
				ComputeRequirements: targetConfig.Guest,
				ComputeImage:        targetConfig.Image,
			}
			vol, err = flapsClient.CreateVolume(ctx, volInput)
			if err != nil {
				return err
			}
		}

		targetConfig.Mounts = []fly.MachineMount{
			{
				Volume:                 vol.ID,
				Path:                   mnt.Path,
				ExtendThresholdPercent: mnt.ExtendThresholdPercent,
				AddSizeGb:              mnt.AddSizeGb,
				SizeGbLimit:            mnt.SizeGbLimit,
			},
		}
	}

	// Standby machine
	if flag.IsSpecified(ctx, "standby-for") {
		standbys := flag.GetStringSlice(ctx, "standby-for")
		for idx := range standbys {
			if standbys[idx] == "source" {
				standbys[idx] = source.ID
			}
		}
		targetConfig.Standbys = lo.Ternary(len(standbys) > 0, standbys, nil)
		targetConfig.Env = lo.Assign(targetConfig.Env,
			map[string]string{"FLY_STANDBY_FOR": strings.Join(standbys, ",")},
		)
	}

	input := fly.LaunchMachineInput{
		Name:       flag.GetString(ctx, "name"),
		Region:     region,
		Config:     targetConfig,
		SkipLaunch: len(targetConfig.Standbys) > 0,
	}

	fmt.Fprintf(out, "Provisioning a new Machine with image %s...\n", source.Config.Image)

	launchedMachine, err := flapsClient.Launch(ctx, input)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "  Machine %s has been created...\n", colorize.Bold(launchedMachine.ID))

	if flag.GetDetach(ctx) {
		return nil
	}

	if !input.SkipLaunch {
		fmt.Fprintf(out, "  Waiting for Machine %s to start...\n", colorize.Bold(launchedMachine.ID))

		// wait for a machine to be started
		err = mach.WaitForStartOrStop(ctx, launchedMachine, "start", time.Minute*5)
		if err != nil {
			return err
		}

		if err = watch.MachinesChecks(ctx, []*fly.Machine{launchedMachine}); err != nil {
			return fmt.Errorf("error while watching health checks: %w", err)
		}
	}

	fmt.Fprintf(out, "Machine has been successfully cloned!\n")

	return
}
