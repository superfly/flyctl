package machine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/shlex"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/exp/slices"
)

func newClone() *cobra.Command {
	const (
		short = "Clone a Fly machine"
		long  = short + "\n"

		usage = "clone <machine_id>"
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
			Description: "Existing volume to attach to the new machine in the form of <volume_id>[:/path/inside/machine]",
		},
		flag.String{
			Name:        "process-group",
			Description: "For machines that are part of Fly Apps v2 does a regular clone and changes the process group to what is specified here",
		},
		flag.String{
			Name:        "override-cmd",
			Description: "Set CMD on the new machine to this value",
		},
		flag.Bool{
			Name:        "clear-cmd",
			Description: "Set empty CMD on the new machine so it uses default CMD for the image",
		},
		flag.Bool{
			Name:        "clear-auto-destroy",
			Description: "Disable auto destroy setting on new machine",
		},
		flag.StringSlice{
			Name:        "standby-for",
			Description: "Comma separated list of machine ids to watch for. You can use '--standby-for=source' to create a standby for the cloned machine",
		},
	)

	return cmd
}

func runMachineClone(ctx context.Context) (err error) {
	var (
		out      = iostreams.FromContext(ctx).Out
		appName  = appconfig.NameFromContext(ctx)
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		client   = client.FromContext(ctx).API()
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	machineID := flag.FirstArg(ctx)
	haveMachineID := len(flag.Args(ctx)) > 0
	source, ctx, err := selectOneMachine(ctx, app, machineID, haveMachineID)
	if err != nil {
		return err
	}
	flapsClient := flaps.FromContext(ctx)

	region := flag.GetString(ctx, "region")
	if region == "" {
		region = source.Region
	}

	fmt.Fprintf(out, "Cloning machine %s into region %s\n", colorize.Bold(source.ID), colorize.Bold(region))

	targetConfig := source.Config
	if targetProcessGroup := flag.GetString(ctx, "process-group"); targetProcessGroup != "" {
		appConfig, err := getAppConfig(ctx, appName)
		if err != nil {
			return fmt.Errorf("failed to get app config: %w", err)
		}

		if !slices.Contains(appConfig.ProcessNames(), targetProcessGroup) {
			return fmt.Errorf("process group %s is not present in app configuration, add a [processes] section to fly.toml", targetProcessGroup)
		}
		if targetProcessGroup == api.MachineProcessGroupFlyAppReleaseCommand {
			return fmt.Errorf("invalid process group %s, %s is reserved for internal use", targetProcessGroup, api.MachineProcessGroupFlyAppReleaseCommand)
		}

		if targetConfig.Metadata == nil {
			targetConfig.Metadata = make(map[string]string)
		}
		targetConfig.Metadata[api.MachineConfigMetadataKeyFlyProcessGroup] = targetProcessGroup

		terminal.Infof("Setting process group to %s for new machine and updating cmd, services, and checks\n", targetProcessGroup)
		mConfig, err := appConfig.ToMachineConfig(targetProcessGroup, nil)
		if err != nil {
			return fmt.Errorf("failed to get process group config: %w", err)
		}
		targetConfig.Init.Cmd = mConfig.Init.Cmd
		targetConfig.Services = mConfig.Services
		targetConfig.Checks = mConfig.Checks
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
		fmt.Fprintf(io.Out, "Auto destroy enabled and will destroy machine on exit. Use --clear-auto-destroy to remove this setting.\n")
	}

	var volID string
	if volumeInfo := flag.GetString(ctx, "attach-volume"); volumeInfo != "" {
		splitVolumeInfo := strings.Split(volumeInfo, ":")

		if len(source.Config.Mounts) > 1 {
			return fmt.Errorf("Can't use --attach-volume for machines with more than 1 volume.")
		} else if len(source.Config.Mounts) == 1 && len(splitVolumeInfo) > 1 {
			return fmt.Errorf("Can't set a mount path on a machine with a volume, please use only the volume id on '%s'", volumeInfo)
		} else if len(source.Config.Mounts) == 0 && len(splitVolumeInfo) != 2 {
			return fmt.Errorf("Couldn't find a mount path on '%s'", volumeInfo)
		}

		// patch the source config so the loop below attaches the volume on the passed mount path
		if len(source.Config.Mounts) == 0 && len(splitVolumeInfo) == 2 {
			volID = splitVolumeInfo[0]
			source.Config.Mounts = []api.MachineMount{
				{
					Path: splitVolumeInfo[1],
				},
			}
		} else {
			volID = splitVolumeInfo[0]
		}
	}

	for _, mnt := range source.Config.Mounts {
		var vol *api.Volume
		if volID != "" {
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
				fmt.Fprintf(io.Out, "Creating new volume from snapshot: %s\n", colorize.Bold(*snapshotID))
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

	// Standby machine
	if flag.IsSpecified(ctx, "standby-for") {
		standbys := flag.GetStringSlice(ctx, "standby-for")
		for idx := range standbys {
			if standbys[idx] == "source" {
				standbys[idx] = source.ID
			}
		}
		targetConfig.Standbys = lo.Ternary(len(standbys) > 0, standbys, nil)
	}

	input := api.LaunchMachineInput{
		AppID:      app.Name,
		Name:       flag.GetString(ctx, "name"),
		Region:     region,
		Config:     targetConfig,
		SkipLaunch: len(targetConfig.Standbys) > 0,
	}

	fmt.Fprintf(out, "Provisioning a new machine with image %s...\n", source.Config.Image)

	launchedMachine, err := flapsClient.Launch(ctx, input)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "  Machine %s has been created...\n", colorize.Bold(launchedMachine.ID))

	if !input.SkipLaunch {
		fmt.Fprintf(out, "  Waiting for machine %s to start...\n", colorize.Bold(launchedMachine.ID))

		// wait for a machine to be started
		err = mach.WaitForStartOrStop(ctx, launchedMachine, "start", time.Minute*5)
		if err != nil {
			return err
		}

		if err = watch.MachinesChecks(ctx, []*api.Machine{launchedMachine}); err != nil {
			return fmt.Errorf("error while watching health checks: %w", err)
		}
	}

	fmt.Fprintf(out, "Machine has been successfully cloned!\n")

	return
}

func getAppConfig(ctx context.Context, appName string) (*appconfig.Config, error) {
	cfg := appconfig.ConfigFromContext(ctx)
	if cfg == nil {
		terminal.Debug("no local app config detected; fetching from backend ...")

		cfg, err := appconfig.FromRemoteApp(ctx, appName)
		if err != nil {
			return nil, fmt.Errorf("failed fetching existing app config: %w", err)
		}

		return cfg, nil
	}

	err, _ := cfg.Validate(ctx)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
