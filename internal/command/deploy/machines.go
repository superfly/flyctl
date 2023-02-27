package deploy

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/google/shlex"
	"github.com/morikuni/aec"
	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appv2"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
)

const (
	DefaultWaitTimeout = 120 * time.Second
	DefaultLeaseTtl    = 13 * time.Second
)

type MachineDeployment interface {
	DeployMachinesApp(context.Context) error
}

type MachineDeploymentArgs struct {
	AppCompact           *api.AppCompact
	DeploymentImage      *imgsrc.DeploymentImage
	Strategy             string
	EnvFromFlags         []string
	PrimaryRegionFlag    string
	AutoConfirmMigration bool
	BuildOnly            bool
	SkipHealthChecks     bool
	RestartOnly          bool
	WaitTimeout          time.Duration
	LeaseTimeout         time.Duration
}

type machineDeployment struct {
	apiClient                  *api.Client
	gqlClient                  graphql.Client
	flapsClient                *flaps.Client
	io                         *iostreams.IOStreams
	colorize                   *iostreams.ColorScheme
	app                        *api.AppCompact
	appConfig                  *appv2.Config
	processConfigs             map[string]*appv2.ProcessConfig
	img                        *imgsrc.DeploymentImage
	machineSet                 machine.MachineSet
	releaseCommandMachine      machine.MachineSet
	releaseCommand             []string
	volumeDestination          string
	volumes                    []api.Volume
	strategy                   string
	releaseId                  string
	releaseVersion             int
	autoConfirmAppsV2Migration bool
	skipHealthChecks           bool
	restartOnly                bool
	waitTimeout                time.Duration
	leaseTimeout               time.Duration
	leaseDelayBetween          time.Duration
}

func NewMachineDeployment(ctx context.Context, args MachineDeploymentArgs) (MachineDeployment, error) {
	if !args.RestartOnly && args.DeploymentImage == nil {
		return nil, fmt.Errorf("BUG: machines deployment created without specifying the image")
	}
	if args.RestartOnly && args.DeploymentImage != nil {
		return nil, fmt.Errorf("BUG: restartOnly machines deployment created and specified an image")
	}
	appConfig, err := determineAppConfigForMachines(ctx, args.EnvFromFlags, args.PrimaryRegionFlag)
	if err != nil {
		return nil, err
	}
	err = appConfig.Validate()
	if err != nil {
		return nil, err
	}
	if args.AppCompact == nil {
		return nil, fmt.Errorf("BUG: args.AppCompact should be set when calling this method")
	}
	flapsClient, err := flaps.New(ctx, args.AppCompact)
	if err != nil {
		return nil, err
	}
	var releaseCmd []string
	if appConfig.Deploy != nil {
		releaseCmd, err = shlex.Split(appConfig.Deploy.ReleaseCommand)
		if err != nil {
			return nil, err
		}
	}
	waitTimeout := args.WaitTimeout
	if waitTimeout == 0 {
		waitTimeout = DefaultWaitTimeout
	}
	leaseTimeout := args.LeaseTimeout
	if leaseTimeout == 0 {
		leaseTimeout = DefaultLeaseTtl
	}
	leaseDelayBetween := (leaseTimeout - 1*time.Second) / 3
	if waitTimeout != DefaultWaitTimeout || leaseTimeout != DefaultLeaseTtl || args.WaitTimeout == 0 || args.LeaseTimeout == 0 {
		terminal.Infof("Using wait timeout: %s lease timeout: %s delay between lease refreshes: %s\n", waitTimeout, leaseTimeout, leaseDelayBetween)
	}
	processConfigs, err := appConfig.GetProcessConfigs()
	if err != nil {
		return nil, err
	}
	io := iostreams.FromContext(ctx)
	apiClient := client.FromContext(ctx).API()
	md := &machineDeployment{
		apiClient:                  apiClient,
		gqlClient:                  apiClient.GenqClient,
		flapsClient:                flapsClient,
		io:                         io,
		colorize:                   io.ColorScheme(),
		app:                        args.AppCompact,
		appConfig:                  appConfig,
		processConfigs:             processConfigs,
		img:                        args.DeploymentImage,
		autoConfirmAppsV2Migration: args.AutoConfirmMigration,
		skipHealthChecks:           args.SkipHealthChecks,
		restartOnly:                args.RestartOnly,
		waitTimeout:                waitTimeout,
		leaseTimeout:               leaseTimeout,
		leaseDelayBetween:          leaseDelayBetween,
		releaseCommand:             releaseCmd,
		releaseCommandMachine:      machine.NewMachineSet(flapsClient, io, nil),
	}
	err = md.setStrategy(args.Strategy)
	if err != nil {
		return nil, err
	}
	err = md.setMachinesForDeployment(ctx)
	if err != nil {
		return nil, err
	}
	err = md.setVolumeConfig(ctx)
	if err != nil {
		return nil, err
	}
	err = md.validateProcessesConfig()
	if err != nil {
		return nil, err
	}
	err = md.validateVolumeConfig()
	if err != nil {
		return nil, err
	}
	err = md.provisionIpsOnFirstDeploy(ctx)
	if err != nil {
		return nil, err
	}
	err = md.createReleaseInBackend(ctx)
	if err != nil {
		return nil, err
	}
	return md, nil
}

func (md *machineDeployment) runReleaseCommand(ctx context.Context) error {
	if len(md.releaseCommand) == 0 || md.restartOnly {
		return nil
	}
	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.ErrOut, "Running %s release_command: %s\n",
		md.colorize.Bold(md.app.Name),
		md.appConfig.Deploy.ReleaseCommand,
	)
	err := md.createOrUpdateReleaseCmdMachine(ctx)
	if err != nil {
		return fmt.Errorf("error running release_command machine: %w", err)
	}
	releaseCmdMachine := md.releaseCommandMachine.GetMachines()[0]
	// FIXME: consolidate this wait stuff with deploy waits? Especially once we improve the outpu
	err = releaseCmdMachine.WaitForState(ctx, api.MachineStateStarted, md.waitTimeout)
	if err != nil {
		return fmt.Errorf("error waiting for release_command machine %s to start: %w", releaseCmdMachine.Machine().ID, err)
	}
	err = releaseCmdMachine.WaitForState(ctx, api.MachineStateDestroyed, md.waitTimeout)
	if err != nil {
		return fmt.Errorf("error waiting for release_command machine %s to finish running: %w", releaseCmdMachine.Machine().ID, err)
	}
	lastExitEvent, err := releaseCmdMachine.WaitForEventTypeAfterType(ctx, "exit", "start", md.waitTimeout)
	if err != nil {
		return fmt.Errorf("error finding the release_command machine %s exit event: %w", releaseCmdMachine.Machine().ID, err)
	}
	exitCode, err := lastExitEvent.Request.GetExitCode()
	if err != nil {
		return fmt.Errorf("error get release_command machine %s exit code: %w", releaseCmdMachine.Machine().ID, err)
	}
	if exitCode != 0 {
		fmt.Fprintf(md.io.ErrOut, "Error release_command failed running on machine %s with exit code %s. Check the logs at: https://fly.io/apps/%s/monitoring\n",
			md.colorize.Bold(releaseCmdMachine.Machine().ID), md.colorize.Red(strconv.Itoa(exitCode)), md.app.Name)
		return fmt.Errorf("error release_command machine %s exited with non-zero status of %d", releaseCmdMachine.Machine().ID, exitCode)
	}
	md.logClearLinesAbove(1)
	fmt.Fprintf(md.io.ErrOut, "  release_command %s completed successfully\n", md.colorize.Bold(releaseCmdMachine.Machine().ID))
	return nil
}

func (md *machineDeployment) DeployMachinesApp(ctx context.Context) error {
	err := md.runReleaseCommand(ctx)
	if err != nil {
		return fmt.Errorf("release command failed - aborting deployment. %w", err)
	}

	if md.machineSet.IsEmpty() {
		return md.createOneMachine(ctx)
	}

	err = md.machineSet.AcquireLeases(ctx, md.leaseTimeout)
	defer func() {
		err := md.machineSet.ReleaseLeases(ctx)
		if err != nil {
			terminal.Warnf("error releasing leases on machines: %v\n", err)
		}
	}()
	if err != nil {
		return err
	}
	md.machineSet.StartBackgroundLeaseRefresh(ctx, md.leaseTimeout, md.leaseDelayBetween)

	// FIXME: handle deploy strategy: rolling, immediate, canary, bluegreen

	fmt.Fprintf(md.io.Out, "Deploying %s app with %s strategy\n", md.colorize.Bold(md.app.Name), md.strategy)
	for _, m := range md.machineSet.GetMachines() {
		launchInput := md.resolveUpdatedMachineConfig(m.Machine())

		fmt.Fprintf(md.io.ErrOut, "  Updating %s\n", md.colorize.Bold(m.FormattedMachineId()))
		err := m.Update(ctx, *launchInput)
		if err != nil {
			if md.strategy != "immediate" {
				return err
			} else {
				fmt.Printf("Continuing after error: %s\n", err)
			}
		}

		if md.strategy != "immediate" {
			err := m.WaitForState(ctx, api.MachineStateStarted, md.waitTimeout)
			if err != nil {
				return err
			}
		}

		if md.strategy != "immediate" && !md.skipHealthChecks {
			err := m.WaitForHealthchecksToPass(ctx, md.waitTimeout)
			// FIXME: combine this wait with the wait for start as one update line (or two per in noninteractive case)
			if err != nil {
				return err
			} else {
				md.logClearLinesAbove(1)
				fmt.Fprintf(md.io.ErrOut, "  Machine %s update finished: %s\n",
					md.colorize.Bold(m.FormattedMachineId()),
					md.colorize.Green("success"),
				)
			}
		}
	}

	fmt.Fprintf(md.io.ErrOut, "  Finished deploying\n")
	return nil
}

func (md *machineDeployment) createOneMachine(ctx context.Context) error {
	fmt.Fprintf(md.io.Out, "No machines in %s app, launching one new machine\n", md.colorize.Bold(md.app.Name))
	launchInput := md.resolveUpdatedMachineConfig(nil)
	newMachineRaw, err := md.flapsClient.Launch(ctx, *launchInput)
	newMachine := machine.NewLeasableMachine(md.flapsClient, md.io, newMachineRaw)
	if err != nil {
		return fmt.Errorf("error creating a new machine machine: %w", err)
	}
	// FIXME: dry this up with release commands and non-empty update
	fmt.Fprintf(md.io.ErrOut, "  Created release_command machine %s\n", md.colorize.Bold(newMachineRaw.ID))
	if md.strategy != "immediate" {
		err := newMachine.WaitForState(ctx, api.MachineStateStarted, md.waitTimeout)
		if err != nil {
			return err
		}
	}
	if md.strategy != "immediate" && !md.skipHealthChecks {
		err := newMachine.WaitForHealthchecksToPass(ctx, md.waitTimeout)
		// FIXME: combine this wait with the wait for start as one update line (or two per in noninteractive case)
		if err != nil {
			return err
		} else {
			md.logClearLinesAbove(1)
			fmt.Fprintf(md.io.ErrOut, "  Machine %s update finished: %s\n",
				md.colorize.Bold(newMachine.FormattedMachineId()),
				md.colorize.Green("success"),
			)
		}
	}
	fmt.Fprintf(md.io.ErrOut, "  Finished deploying\n")
	return nil
}

func (md *machineDeployment) setMachinesForDeployment(ctx context.Context) error {
	machines, releaseCmdMachine, err := md.flapsClient.ListFlyAppsMachines(ctx)
	if err != nil {
		return err
	}

	// migrate non-platform machines into fly platform
	if len(machines) == 0 {
		terminal.Debug("Found no machines that are part of Fly Apps Platform. Check for other machines...")
		machines, err = md.flapsClient.ListActive(ctx)
		if err != nil {
			return err
		}
		if len(machines) > 0 {
			rows := make([][]string, 0)
			for _, machine := range machines {
				var volName string
				if machine.Config != nil && len(machine.Config.Mounts) > 0 {
					volName = machine.Config.Mounts[0].Volume
				}

				rows = append(rows, []string{
					machine.ID,
					machine.Name,
					machine.State,
					machine.Region,
					machine.ImageRefWithVersion(),
					machine.PrivateIP,
					volName,
					machine.CreatedAt,
					machine.UpdatedAt,
				})
			}
			terminal.Warnf("Found %d machines that are not part of the Fly Apps Platform:\n", len(machines))
			_ = render.Table(iostreams.FromContext(ctx).Out, fmt.Sprintf("%s machines", md.app.Name), rows, "ID", "Name", "State", "Region", "Image", "IP Address", "Volume", "Created", "Last Updated")
			if !md.autoConfirmAppsV2Migration {
				switch confirmed, err := prompt.Confirmf(ctx, "Migrate %d existing machines into Fly Apps Platform?", len(machines)); {
				case err == nil:
					if !confirmed {
						terminal.Info("Skipping machines migration to Fly Apps Platform and the deployment")
						md.machineSet = machine.NewMachineSet(md.flapsClient, md.io, nil)
						return nil
					}
				case prompt.IsNonInteractive(err):
					return prompt.NonInteractiveError("not running interactively, use --auto-confirm flag to confirm")
				default:
					return err
				}
			}
			terminal.Infof("Migrating %d machines to the Fly Apps Platform\n", len(machines))
		}
	}

	md.machineSet = machine.NewMachineSet(md.flapsClient, md.io, machines)
	var releaseCmdSet []*api.Machine
	if releaseCmdMachine != nil {
		releaseCmdSet = []*api.Machine{releaseCmdMachine}
	}
	md.releaseCommandMachine = machine.NewMachineSet(md.flapsClient, md.io, releaseCmdSet)
	return nil
}

func (md *machineDeployment) createOrUpdateReleaseCmdMachine(ctx context.Context) error {
	if md.releaseCommandMachine.IsEmpty() {
		err := md.createReleaseCommandMachine(ctx)
		if err != nil {
			return err
		}
	} else {
		err := md.updateReleaseCommandMachine(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (md *machineDeployment) configureLaunchInputForReleaseCommand(launchInput *api.LaunchMachineInput) *api.LaunchMachineInput {
	launchInput.Config.Init.Cmd = md.releaseCommand
	launchInput.Config.Metadata[api.MachineConfigMetadataKeyFlyProcessGroup] = api.MachineProcessGroupFlyAppReleaseCommand
	launchInput.Config.Services = nil
	launchInput.Config.Checks = nil
	launchInput.Config.Restart = api.MachineRestart{
		Policy: api.MachineRestartPolicyNo,
	}
	launchInput.Config.AutoDestroy = true
	launchInput.Config.DNS = &api.DNSConfig{SkipRegistration: true}
	if md.appConfig.PrimaryRegion != "" {
		launchInput.Region = md.appConfig.PrimaryRegion
	}
	if _, present := launchInput.Config.Env["RELEASE_COMMAND"]; !present {
		launchInput.Config.Env["RELEASE_COMMAND"] = "1"
	}
	return launchInput
}

func (md *machineDeployment) createReleaseCommandMachine(ctx context.Context) error {
	if len(md.releaseCommand) == 0 || !md.releaseCommandMachine.IsEmpty() {
		return nil
	}
	launchInput := md.resolveUpdatedMachineConfig(nil)
	launchInput = md.configureLaunchInputForReleaseCommand(launchInput)
	releaseCmdMachine, err := md.flapsClient.Launch(ctx, *launchInput)
	if err != nil {
		return fmt.Errorf("error creating a release_command machine: %w", err)
	}
	fmt.Fprintf(md.io.ErrOut, "  Created release_command machine %s\n", md.colorize.Bold(releaseCmdMachine.ID))
	md.releaseCommandMachine = machine.NewMachineSet(md.flapsClient, md.io, []*api.Machine{releaseCmdMachine})
	return nil
}

func (md *machineDeployment) updateReleaseCommandMachine(ctx context.Context) error {
	if len(md.releaseCommand) == 0 {
		return nil
	}
	if md.releaseCommandMachine.IsEmpty() {
		return fmt.Errorf("expected release_command machine to exist already, but it does not :-(")
	}
	releaseCmdMachine := md.releaseCommandMachine.GetMachines()[0]
	fmt.Fprintf(md.io.ErrOut, "  Updating release_command machine %s\n", md.colorize.Bold(releaseCmdMachine.Machine().ID))
	err := releaseCmdMachine.WaitForState(ctx, api.MachineStateStopped, md.waitTimeout)
	if err != nil {
		return err
	}
	updatedConfig := md.resolveUpdatedMachineConfig(releaseCmdMachine.Machine())
	updatedConfig = md.configureLaunchInputForReleaseCommand(updatedConfig)
	err = md.releaseCommandMachine.AcquireLeases(ctx, md.leaseTimeout)
	defer func() {
		_ = md.releaseCommandMachine.ReleaseLeases(ctx)
	}()
	if err != nil {
		return err
	}
	err = releaseCmdMachine.Update(ctx, *updatedConfig)
	if err != nil {
		return fmt.Errorf("error updating release_command machine: %w", err)
	}
	return nil
}

func (md *machineDeployment) setVolumeConfig(ctx context.Context) error {
	if md.appConfig.Mounts == nil {
		return nil
	}

	md.volumeDestination = md.appConfig.Mounts.Destination

	if volumes, err := md.apiClient.GetVolumes(ctx, md.app.Name); err != nil {
		return fmt.Errorf("Error fetching application volumes: %w", err)
	} else {
		md.volumes = lo.Filter(volumes, func(v api.Volume, _ int) bool {
			return v.Name == md.appConfig.Mounts.Source && v.AttachedAllocation == nil && v.AttachedMachine == nil
		})
	}
	return nil
}

func (md *machineDeployment) validateProcessesConfig() error {
	appConfigProcessesExist := md.appConfig.Processes != nil && len(md.appConfig.Processes) > 0
	appConfigProcessesStr := ""
	first := true
	for procGroupName := range md.appConfig.Processes {
		if !first {
			appConfigProcessesStr += ", "
		} else {
			first = false
		}
		appConfigProcessesStr += procGroupName
	}
	for _, m := range md.machineSet.GetMachines() {
		mid := m.Machine().ID
		machineProcGroup := m.Machine().Config.Metadata[api.MachineConfigMetadataKeyFlyProcessGroup]
		machineProcGroupPresent := machineProcGroup != ""
		if machineProcGroup == api.MachineProcessGroupFlyAppReleaseCommand {
			continue
		}
		// we put the api.MachineProcessGroupApp process group on machine by default
		if !appConfigProcessesExist && machineProcGroup == api.MachineProcessGroupApp {
			continue
		}
		if !machineProcGroupPresent && appConfigProcessesExist {
			return fmt.Errorf("error machine %s does not have a process group and should have one from app configuration: %s", mid, appConfigProcessesStr)
		}
		if machineProcGroupPresent && !appConfigProcessesExist {
			return fmt.Errorf("error machine %s has process group '%s' and no processes are defined in app config; add [processes] to fly.toml or remove the process group from this machine", mid, machineProcGroup)
		}
		if machineProcGroupPresent {
			_, appConfigProcGroupPresent := md.appConfig.Processes[machineProcGroup]
			if !appConfigProcGroupPresent {
				return fmt.Errorf("error machine %s has process group '%s', which is missing from the processes in the app config: %s", mid, machineProcGroup, appConfigProcessesStr)
			}
		}
	}
	return nil
}

func (md *machineDeployment) validateVolumeConfig() error {
	for _, m := range md.machineSet.GetMachines() {
		mid := m.Machine().ID
		if m.Machine().Config.Metadata[api.MachineConfigMetadataKeyFlyProcessGroup] == api.MachineProcessGroupFlyAppReleaseCommand {
			continue
		}
		mountsConfig := m.Machine().Config.Mounts
		if len(mountsConfig) > 1 {
			return fmt.Errorf("error machine %s has %d mounts and expected 1", mid, len(mountsConfig))
		}
		if md.volumeDestination == "" && len(mountsConfig) != 0 {
			return fmt.Errorf("error machine %s has a volume mounted and app config does not specify a volume; remove the volume from the machine or add a [mounts] configuration to fly.toml", mid)
		}
		if md.volumeDestination != "" && len(mountsConfig) == 0 {
			return fmt.Errorf("error machine %s does not have a volume configured and fly.toml expects one with destination %s; remove the [mounts] configuration in fly.toml or use the machines API to add a volume to this machine", mid, md.volumeDestination)
		}
	}

	if md.machineSet.IsEmpty() && md.volumeDestination != "" && len(md.volumes) == 0 {
		return fmt.Errorf("error new machine requires an unattached volume named '%s' on mount destination '%s'",
			md.appConfig.Mounts.Source, md.volumeDestination)
	}
	return nil
}

func (md *machineDeployment) setStrategy(passedInStrategy string) error {
	if passedInStrategy != "" {
		md.strategy = passedInStrategy
	} else if md.appConfig.Deploy != nil && md.appConfig.Deploy.Strategy != "" {
		md.strategy = md.appConfig.Deploy.Strategy
	} else {
		md.strategy = "rolling"
	}
	if md.strategy != "rolling" && md.strategy != "immediate" {
		return fmt.Errorf("error unsupported deployment strategy '%s'; fly deploy for machines supports rolling and immediate strategies", md.strategy)
	}
	return nil
}

func (md *machineDeployment) createReleaseInBackend(ctx context.Context) error {
	_ = `# @genqlient
	mutation MachinesCreateRelease($input:CreateReleaseInput!) {
		createRelease(input:$input) {
			release {
				id
				version
			}
		}
	}
	`
	input := gql.CreateReleaseInput{
		AppId:           md.appConfig.AppName,
		PlatformVersion: "machines",
		Strategy:        gql.DeploymentStrategy(strings.ToUpper(md.strategy)),
		Definition:      md.appConfig,
	}
	if !md.restartOnly {
		input.Image = md.img.Tag
	} else if !md.machineSet.IsEmpty() {
		input.Image = md.machineSet.GetMachines()[0].Machine().Config.Image
	}
	resp, err := gql.MachinesCreateRelease(ctx, md.gqlClient, input)
	if err != nil {
		return err
	}
	md.releaseId = resp.CreateRelease.Release.Id
	md.releaseVersion = resp.CreateRelease.Release.Version
	return nil
}

func (md *machineDeployment) resolveUpdatedMachineConfig(origMachineRaw *api.Machine) *api.LaunchMachineInput {
	if origMachineRaw == nil {
		origMachineRaw = &api.Machine{
			Region: md.appConfig.PrimaryRegion,
			Config: &api.MachineConfig{},
		}
	}
	machineConf := &api.MachineConfig{}
	if md.restartOnly {
		machineConf = origMachineRaw.Config
	}
	launchInput := &api.LaunchMachineInput{
		ID:      origMachineRaw.ID,
		AppID:   md.app.Name,
		OrgSlug: md.app.Organization.ID,
		Config:  machineConf,
		Region:  origMachineRaw.Region,
	}
	launchInput.Config.Metadata = md.defaultMachineMetadata()
	if origMachineRaw.Config.Metadata != nil {
		for k, v := range origMachineRaw.Config.Metadata {
			if !isFlyAppsPlatformMetadata(k) {
				launchInput.Config.Metadata[k] = v
			}
		}
	}
	if md.restartOnly {
		return launchInput
	}

	launchInput.Config.Image = md.img.Tag
	launchInput.Config.Metrics = md.appConfig.Metrics
	launchInput.Config.Restart = origMachineRaw.Config.Restart
	for _, s := range md.appConfig.Statics {
		launchInput.Config.Statics = append(launchInput.Config.Statics, &api.Static{
			GuestPath: s.GuestPath,
			UrlPrefix: s.UrlPrefix,
		})
	}
	launchInput.Config.Env = make(map[string]string)
	for k, v := range md.appConfig.Env {
		launchInput.Config.Env[k] = v
	}
	if launchInput.Config.Env["PRIMARY_REGION"] == "" && origMachineRaw.Config.Env["PRIMARY_REGION"] != "" {
		launchInput.Config.Env["PRIMARY_REGION"] = origMachineRaw.Config.Env["PRIMARY_REGION"]
	}

	if origMachineRaw.Config.Mounts != nil {
		launchInput.Config.Mounts = origMachineRaw.Config.Mounts
	} else if md.appConfig.Mounts != nil {
		launchInput.Config.Mounts = []api.MachineMount{{
			Path:   md.volumeDestination,
			Volume: md.volumes[0].ID,
		}}
	}

	if len(launchInput.Config.Mounts) == 1 && launchInput.Config.Mounts[0].Path != md.volumeDestination {
		currentMount := launchInput.Config.Mounts[0]
		terminal.Warnf("Updating the mount path for volume %s on machine %s from %s to %s due to fly.toml [mounts] destination value\n", currentMount.Volume, origMachineRaw.ID, currentMount.Path, md.volumeDestination)
		launchInput.Config.Mounts[0].Path = md.volumeDestination
	}
	if origMachineRaw.Config.Guest != nil {
		launchInput.Config.Guest = origMachineRaw.Config.Guest
	}
	launchInput.Config.Init = origMachineRaw.Config.Init
	processGroup := origMachineRaw.Config.Metadata[api.MachineConfigMetadataKeyFlyProcessGroup]
	if processGroup == "" {
		processGroup = api.MachineProcessGroupApp
	}
	if processConfig, ok := md.processConfigs[processGroup]; ok {
		launchInput.Config.Services = processConfig.Services
		launchInput.Config.Init.Cmd = processConfig.Cmd
		launchInput.Config.Checks = processConfig.Checks
	}
	return launchInput
}

func (md *machineDeployment) defaultMachineMetadata() map[string]string {
	res := map[string]string{
		api.MachineConfigMetadataKeyFlyPlatformVersion: api.MachineFlyPlatformVersion2,
		api.MachineConfigMetadataKeyFlyReleaseId:       md.releaseId,
		api.MachineConfigMetadataKeyFlyReleaseVersion:  strconv.Itoa(md.releaseVersion),
		api.MachineConfigMetadataKeyFlyProcessGroup:    api.MachineProcessGroupApp,
	}
	if md.app.IsPostgresApp() {
		res[api.MachineConfigMetadataKeyFlyManagedPostgres] = "true"
	}
	return res
}

func isFlyAppsPlatformMetadata(key string) bool {
	return key == api.MachineConfigMetadataKeyFlyPlatformVersion ||
		key == api.MachineConfigMetadataKeyFlyReleaseId ||
		key == api.MachineConfigMetadataKeyFlyReleaseVersion ||
		key == api.MachineConfigMetadataKeyFlyManagedPostgres
}

func (md *machineDeployment) provisionIpsOnFirstDeploy(ctx context.Context) error {
	if md.app.Deployed || !md.machineSet.IsEmpty() {
		return nil
	}
	if md.appConfig.HttpService != nil || len(md.appConfig.Services) > 0 {
		ipAddrs, err := md.apiClient.GetIPAddresses(ctx, md.app.Name)
		if err != nil {
			return fmt.Errorf("error detecting ip addresses allocated to %s app: %w", md.app.Name, err)
		}
		if len(ipAddrs) > 0 {
			return nil
		}
		fmt.Fprintf(md.io.Out, "Provisioning ips for %s\n", md.colorize.Bold(md.app.Name))
		v6Addr, err := md.apiClient.AllocateIPAddress(ctx, md.app.Name, "v6", "", nil, "")
		if err != nil {
			return fmt.Errorf("error allocating ipv6 after detecting first deploy and presence of services: %w", err)
		}
		fmt.Fprintf(md.io.Out, "  Dedicated ipv6: %s\n", v6Addr.Address)
		v4Shared, err := md.apiClient.AllocateSharedIPAddress(ctx, md.app.Name)
		if err != nil {
			return fmt.Errorf("error allocating shared ipv4 after detecting first deploy and presence of services: %w", err)
		}
		fmt.Fprintf(md.io.Out, "  Shared ipv4: %s\n", v4Shared)
		fmt.Fprintf(md.io.Out, "  Add a dedicated ipv4 with: fly ips allocate-v4\n")
	}
	return nil
}

func (md *machineDeployment) logClearLinesAbove(count int) {
	if md.io.IsInteractive() {
		builder := aec.EmptyBuilder
		str := builder.Up(uint(count)).EraseLine(aec.EraseModes.All).ANSI
		fmt.Fprint(md.io.ErrOut, str.String())
	}
}

func determineAppConfigForMachines(ctx context.Context, envFromFlags []string, primaryRegion string) (cfg *appv2.Config, err error) {
	client := client.FromContext(ctx).API()
	appNameFromContext := appv2.NameFromContext(ctx)
	if cfg = appv2.ConfigFromContext(ctx); cfg == nil {
		logger := logger.FromContext(ctx)
		logger.Debug("no local app config detected for machines deploy; fetching from backend ...")

		var apiConfig *api.AppConfig
		if apiConfig, err = client.GetConfig(ctx, appNameFromContext); err != nil {
			err = fmt.Errorf("failed fetching existing app config: %w", err)
			return
		}

		basicApp, err := client.GetAppBasic(ctx, appNameFromContext)
		if err != nil {
			return nil, err
		}

		cfg, err = appv2.FromDefinition(&apiConfig.Definition)
		if err != nil {
			return nil, err
		}
		cfg.AppName = basicApp.Name
	}

	if len(envFromFlags) > 0 {
		var parsedEnv map[string]string
		if parsedEnv, err = cmdutil.ParseKVStringsToMap(envFromFlags); err != nil {
			err = fmt.Errorf("failed parsing environment: %w", err)

			return
		}
		cfg.SetEnvVariables(parsedEnv)
	}

	// deleting this block will result in machines not being deployed in the user selected region
	if primaryRegion != "" {
		cfg.PrimaryRegion = primaryRegion
	}

	// Always prefer the app name passed via --app

	if appNameFromContext != "" {
		cfg.AppName = appNameFromContext
	}

	return
}
