package machine

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
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
	"github.com/superfly/flyctl/terminal"
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
		flag.String{
			Name:        "process-group",
			Description: "For machines that are part of Fly Apps v2 does a regular clone and changes the process group to what is specified here",
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
		return err
	}
	appConfig, err := getAppConfig(ctx, appName)

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
	if targetProcessGroup := flag.GetString(ctx, "process-group"); targetProcessGroup != "" {
		allCmdAndServices, err := appConfig.GetProcessNamesToCmdAndService()
		if err != nil {
			return err
		}
		cmdAndServices, present := allCmdAndServices[targetProcessGroup]
		if !present {
			return fmt.Errorf("process group %s is not present in app configuration, add a [processes] section to fly.toml", targetProcessGroup)
		}
		if targetProcessGroup == api.MachineProcessGroupFlyAppReleaseCommand {
			return fmt.Errorf("invalid process group %s, %s is reserved for internal use", targetProcessGroup, api.MachineProcessGroupFlyAppReleaseCommand)
		}
		targetConfig.Metadata[api.MachineConfigMetadataKeyFlyProcessGroup] = targetProcessGroup
		terminal.Infof("Setting process group to %s for new machine and updating cmd and services\n", targetProcessGroup)
		targetConfig.Init.Cmd = cmdAndServices.Cmd
		targetConfig.Services = cmdAndServices.MachineServices
	}
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

func getAppConfig(ctx context.Context, appName string) (*app.Config, error) {
	apiClient := client.FromContext(ctx).API()
	cfg := app.ConfigFromContext(ctx)
	if cfg == nil {
		terminal.Debug("no local app config detected; fetching from backend ...")

		apiConfig, err := apiClient.GetConfig(ctx, appName)
		if err != nil {
			return nil, fmt.Errorf("failed fetching existing app config: %w", err)
		}

		basicApp, err := apiClient.GetAppBasic(ctx, appName)
		if err != nil {
			return nil, err
		}

		cfg = &app.Config{
			Definition: apiConfig.Definition,
		}

		cfg.AppName = basicApp.Name
		cfg.SetPlatformVersion(basicApp.PlatformVersion)
		return cfg, nil
	} else {
		parsedCfg, err := apiClient.ParseConfig(ctx, appName, cfg.Definition)
		if err != nil {
			return nil, err
		}
		if !parsedCfg.Valid {
			fmt.Println()
			if len(parsedCfg.Errors) > 0 {
				terminal.Errorf("\nConfiguration errors in %s:\n\n", cfg.Path)
			}
			for _, e := range parsedCfg.Errors {
				terminal.Errorf("   %s\n", e)
			}
			fmt.Println()
			return nil, errors.New("error app configuration is not valid")
		}
		return cfg, nil
	}
}
