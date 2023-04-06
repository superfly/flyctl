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
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/cmdutil"
	machcmd "github.com/superfly/flyctl/internal/command/machine"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/machine"
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

type ProcessGroupsDiff struct {
	machinesToRemove      []machine.LeasableMachine
	groupsToRemove        map[string]int
	groupsNeedingMachines map[string]*appconfig.ProcessConfig
}

type MachineDeploymentArgs struct {
	AppCompact        *api.AppCompact
	DeploymentImage   string
	Strategy          string
	EnvFromFlags      []string
	PrimaryRegionFlag string
	BuildOnly         bool
	SkipHealthChecks  bool
	RestartOnly       bool
	WaitTimeout       time.Duration
	LeaseTimeout      time.Duration
}

type machineDeployment struct {
	apiClient             *api.Client
	gqlClient             graphql.Client
	flapsClient           *flaps.Client
	io                    *iostreams.IOStreams
	colorize              *iostreams.ColorScheme
	app                   *api.AppCompact
	appConfig             *appconfig.Config
	processConfigs        map[string]*appconfig.ProcessConfig
	img                   string
	machineSet            machine.MachineSet
	releaseCommandMachine machine.MachineSet
	volumeDestination     string
	volumes               []api.Volume
	strategy              string
	releaseId             string
	releaseVersion        int
	skipHealthChecks      bool
	restartOnly           bool
	waitTimeout           time.Duration
	leaseTimeout          time.Duration
	leaseDelayBetween     time.Duration
}

func NewMachineDeployment(ctx context.Context, args MachineDeploymentArgs) (MachineDeployment, error) {
	if !args.RestartOnly && args.DeploymentImage == "" {
		return nil, fmt.Errorf("BUG: machines deployment created without specifying the image")
	}
	if args.RestartOnly && args.DeploymentImage != "" {
		return nil, fmt.Errorf("BUG: restartOnly machines deployment created and specified an image")
	}
	appConfig, err := determineAppConfigForMachines(ctx, args.EnvFromFlags, args.PrimaryRegionFlag)
	if err != nil {
		return nil, err
	}
	err, _ = appConfig.Validate(ctx)
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
	if appConfig.Deploy != nil {
		_, err = shlex.Split(appConfig.Deploy.ReleaseCommand)
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
		apiClient:         apiClient,
		gqlClient:         apiClient.GenqClient,
		flapsClient:       flapsClient,
		io:                io,
		colorize:          io.ColorScheme(),
		app:               args.AppCompact,
		appConfig:         appConfig,
		processConfigs:    processConfigs,
		img:               args.DeploymentImage,
		skipHealthChecks:  args.SkipHealthChecks,
		restartOnly:       args.RestartOnly,
		waitTimeout:       waitTimeout,
		leaseTimeout:      leaseTimeout,
		leaseDelayBetween: leaseDelayBetween,
	}
	err = md.setStrategy(args.Strategy)
	if err != nil {
		return nil, err
	}
	err = md.setMachinesForDeployment(ctx)
	if err != nil {
		return nil, err
	}
	err = md.setImg(ctx)
	if err != nil {
		return nil, err
	}
	err = md.setVolumeConfig(ctx)
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

func (md *machineDeployment) resolveProcessGroupChanges() ProcessGroupsDiff {
	output := ProcessGroupsDiff{
		groupsToRemove:        map[string]int{},
		groupsNeedingMachines: map[string]*appconfig.ProcessConfig{},
	}

	groupHasMachine := map[string]bool{}

	for _, leasableMachine := range md.machineSet.GetMachines() {
		mach := leasableMachine.Machine()
		machGroup := mach.ProcessGroup()
		groupMatch := ""
		for name := range md.processConfigs {
			if machGroup == name {
				groupMatch = machGroup
				break
			}
		}
		if groupMatch == "" {
			output.groupsToRemove[machGroup] += 1
			output.machinesToRemove = append(output.machinesToRemove, leasableMachine)
		} else {
			groupHasMachine[groupMatch] = true
		}
	}

	for name, val := range md.processConfigs {
		if hasMach, ok := groupHasMachine[name]; !ok || !hasMach {
			output.groupsNeedingMachines[name] = val
		}
	}

	return output
}

func (md *machineDeployment) warnAboutProcessGroupChanges(ctx context.Context, diff ProcessGroupsDiff) {
	var (
		io                 = iostreams.FromContext(ctx)
		colorize           = io.ColorScheme()
		willAddMachines    = len(diff.groupsNeedingMachines) != 0
		willRemoveMachines = diff.machinesToRemove != nil
	)

	if !willAddMachines && !willRemoveMachines {
		return
	}

	fmt.Fprintln(io.Out, "Process groups have changed. This will:")

	if willRemoveMachines {
		bullet := colorize.Red("*")
		for grp, numMach := range diff.groupsToRemove {
			pluralS := lo.Ternary(numMach == 1, "", "s")
			fmt.Fprintf(io.Out, " %s destroy %d \"%s\" machine%s\n", bullet, numMach, grp, pluralS)
		}
	}
	if willAddMachines {
		bullet := colorize.Green("*")

		for name := range diff.groupsNeedingMachines {
			fmt.Fprintf(io.Out, " %s create 1 \"%s\" machine\n", bullet, name)
		}
	}
	fmt.Fprint(io.Out, "\n")
}

func (md *machineDeployment) spawnMachineInGroup(ctx context.Context, groupName string) error {
	if groupName == "" {
		// If the group is unspecified, it should have been translated to "app" by this point
		panic("spawnMachineInGroup requires a non-empty group name. this is a bug!")
	}
	fmt.Fprintf(md.io.Out, "No machines in group '%s', launching one new machine\n", md.colorize.Bold(groupName))
	machBase := &api.Machine{
		Region: md.appConfig.PrimaryRegion,
		Config: &api.MachineConfig{
			Metadata: map[string]string{
				api.MachineConfigMetadataKeyFlyProcessGroup: groupName,
			},
		},
	}
	launchInput := md.resolveUpdatedMachineConfig(machBase, false)
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
	return nil
}

func (md *machineDeployment) DeployMachinesApp(ctx context.Context) error {
	ctx = flaps.NewContext(ctx, md.flapsClient)
	if md.restartOnly {
		return md.restartMachinesApp(ctx)
	}
	return md.deployMachinesApp(ctx)
}

func (md *machineDeployment) restartMachinesApp(ctx context.Context) error {
	err := md.machineSet.AcquireLeases(ctx, md.leaseTimeout)
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

	return md.updateExistingMachines(ctx)
}

func (md *machineDeployment) deployMachinesApp(ctx context.Context) error {
	err := md.runReleaseCommand(ctx)
	if err != nil {
		return fmt.Errorf("release command failed - aborting deployment. %w", err)
	}

	if md.machineSet.IsEmpty() {
		processGroupMachineDiff := ProcessGroupsDiff{
			groupsToRemove:        map[string]int{},
			groupsNeedingMachines: md.processConfigs,
		}
		md.warnAboutProcessGroupChanges(ctx, processGroupMachineDiff)
		for name := range md.processConfigs {
			if err := md.spawnMachineInGroup(ctx, name); err != nil {
				return err
			}
		}
		return nil
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

	processGroupMachineDiff := md.resolveProcessGroupChanges()
	md.warnAboutProcessGroupChanges(ctx, processGroupMachineDiff)

	// Destroy machines that don't fit the current process groups
	if err := md.machineSet.RemoveMachines(ctx, processGroupMachineDiff.machinesToRemove); err != nil {
		return err
	}
	for _, mach := range processGroupMachineDiff.machinesToRemove {
		if err := machcmd.Destroy(ctx, md.app, mach.Machine(), true); err != nil {
			return err
		}
	}

	// Create machines for new process groups
	for name := range processGroupMachineDiff.groupsNeedingMachines {
		if err := md.spawnMachineInGroup(ctx, name); err != nil {
			return err
		}
	}
	if len(processGroupMachineDiff.groupsNeedingMachines) > 0 {
		fmt.Fprintf(md.io.ErrOut, "Finished launching new machines\n")
	}

	return md.updateExistingMachines(ctx)
}

func (md *machineDeployment) updateExistingMachines(ctx context.Context) error {
	// FIXME: handle deploy strategy: rolling, immediate, canary, bluegreen
	fmt.Fprintf(md.io.Out, "Updating existing machines in '%s' with %s strategy\n", md.colorize.Bold(md.app.Name), md.strategy)
	for _, m := range md.machineSet.GetMachines() {
		launchInput := md.resolveUpdatedMachineConfig(m.Machine(), false)

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

func (md *machineDeployment) setMachinesForDeployment(ctx context.Context) error {
	machines, releaseCmdMachine, err := md.flapsClient.ListFlyAppsMachines(ctx)
	if err != nil {
		return err
	}

	// migrate non-platform machines into fly platform
	if len(machines) == 0 {
		terminal.Debug("Found no machines that are part of Fly Apps Platform. Checking for active machines...")
		activeMachines, err := md.flapsClient.ListActive(ctx)
		if err != nil {
			return err
		}
		if len(activeMachines) > 0 {
			return fmt.Errorf(
				"found %d machines that are unmanaged. `fly deploy` only updates machines with %s=%s in their metadata. Use `fly machine list` to list machines and `fly machine update --metadata %s=%s` to update individual machines with the metadata. Once done, `fly deploy` will update machines with the metadata based on your %s app configuration",
				len(activeMachines),
				api.MachineConfigMetadataKeyFlyPlatformVersion,
				api.MachineFlyPlatformVersion2,
				api.MachineConfigMetadataKeyFlyPlatformVersion,
				api.MachineFlyPlatformVersion2,
				appconfig.DefaultConfigFileName,
			)
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

func (md *machineDeployment) validateVolumeConfig() error {
	for _, m := range md.machineSet.GetMachines() {
		mid := m.Machine().ID
		if m.Machine().ProcessGroup() == api.MachineProcessGroupFlyAppReleaseCommand {
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

func (md *machineDeployment) setImg(ctx context.Context) error {
	if md.img != "" {
		return nil
	}
	latestImg, err := md.latestImage(ctx)
	if err == nil {
		md.img = latestImg
		return nil
	}
	if !md.machineSet.IsEmpty() {
		md.img = md.machineSet.GetMachines()[0].Machine().Config.Image
		return nil
	}
	return fmt.Errorf("could not find image to use for deployment; backend error was: %w", err)
}

func (md *machineDeployment) latestImage(ctx context.Context) (string, error) {
	_ = `# @genqlient
	       query FlyctlDeployGetLatestImage($appName:String!) {
	               app(name:$appName) {
	                       currentReleaseUnprocessed {
	                               id
	                               version
	                               imageRef
	                       }
	               }
	       }
	      `
	resp, err := gql.FlyctlDeployGetLatestImage(ctx, md.gqlClient, md.app.Name)
	if err != nil {
		return "", err
	}
	if resp.App.CurrentReleaseUnprocessed.ImageRef == "" {
		return "", fmt.Errorf("current release not found for app %s", md.app.Name)
	}
	return resp.App.CurrentReleaseUnprocessed.ImageRef, nil
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
		Image:           md.img,
	}
	resp, err := gql.MachinesCreateRelease(ctx, md.gqlClient, input)
	if err != nil {
		return err
	}
	md.releaseId = resp.CreateRelease.Release.Id
	md.releaseVersion = resp.CreateRelease.Release.Version
	return nil
}

func (md *machineDeployment) computeMachineConfigMetadata(config *api.MachineConfig) map[string]string {
	return lo.Assign(
		md.defaultMachineMetadata(),
		lo.OmitBy(config.Metadata, func(k, v string) bool {
			return isFlyAppsPlatformMetadata(k)
		}))
}

func (md *machineDeployment) launchInputForRestart(origMachineRaw *api.Machine) *api.LaunchMachineInput {
	if origMachineRaw == nil {
		return nil
	}

	Config := machine.CloneConfig(origMachineRaw.Config)
	Config.Metadata = md.computeMachineConfigMetadata(Config)

	return &api.LaunchMachineInput{
		ID:      origMachineRaw.ID,
		AppID:   md.app.Name,
		OrgSlug: md.app.Organization.ID,
		Config:  Config,
		Region:  origMachineRaw.Region,
	}
}

func (md *machineDeployment) launchInputForReleaseCommand(origMachineRaw *api.Machine) *api.LaunchMachineInput {
	if origMachineRaw == nil {
		origMachineRaw = &api.Machine{
			Region: md.appConfig.PrimaryRegion,
		}
	}
	// We can ignore the error because ToReleaseMachineConfig fails only
	// if it can't split the command and we test that at initialization
	mConfig, _ := md.appConfig.ToReleaseMachineConfig()
	md.setMachineReleaseData(mConfig)

	return &api.LaunchMachineInput{
		ID:      origMachineRaw.ID,
		AppID:   md.app.Name,
		OrgSlug: md.app.Organization.ID,
		Config:  mConfig,
		Region:  origMachineRaw.Region,
	}
}

func (md *machineDeployment) setMachineReleaseData(mConfig *api.MachineConfig) {
	mConfig.Image = md.img
	mConfig.Metadata = md.computeMachineConfigMetadata(mConfig)
}

func (md *machineDeployment) resolveUpdatedMachineConfig(origMachineRaw *api.Machine, forReleaseCommand bool) *api.LaunchMachineInput {
	if md.restartOnly {
		return md.launchInputForRestart(origMachineRaw)
	}
	if forReleaseCommand {
		launchInput := md.launchInputForReleaseCommand(origMachineRaw)
		return md.configureLaunchInputForReleaseCommand(launchInput)
	}

	if origMachineRaw == nil {
		origMachineRaw = &api.Machine{
			Region: md.appConfig.PrimaryRegion,
			Config: &api.MachineConfig{},
		}
	}

	launchInput := &api.LaunchMachineInput{
		ID:      origMachineRaw.ID,
		AppID:   md.app.Name,
		OrgSlug: md.app.Organization.ID,
		Config:  machine.CloneConfig(origMachineRaw.Config),
		Region:  origMachineRaw.Region,
	}

	md.setMachineReleaseData(launchInput.Config)
	launchInput.Config.Env = lo.Assign(md.appConfig.Env)
	if md.appConfig.PrimaryRegion != "" {
		launchInput.Config.Env["PRIMARY_REGION"] = md.appConfig.PrimaryRegion
	}

	// Anything below this point doesn't apply to machines created to run ReleaseCommand
	launchInput.Config.Metrics = md.appConfig.Metrics

	for _, s := range md.appConfig.Statics {
		launchInput.Config.Statics = append(launchInput.Config.Statics, &api.Static{
			GuestPath: s.GuestPath,
			UrlPrefix: s.UrlPrefix,
		})
	}

	if launchInput.Config.Mounts == nil && md.appConfig.Mounts != nil {
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

	processGroup := launchInput.Config.ProcessGroup()
	if processConfig, ok := md.processConfigs[processGroup]; ok {
		launchInput.Config.Services = processConfig.Services
		launchInput.Config.Checks = processConfig.Checks
		launchInput.Config.Init.Cmd = processConfig.Cmd
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

func determineAppConfigForMachines(ctx context.Context, envFromFlags []string, primaryRegion string) (cfg *appconfig.Config, err error) {
	appNameFromContext := appconfig.NameFromContext(ctx)
	if cfg = appconfig.ConfigFromContext(ctx); cfg == nil {
		logger := logger.FromContext(ctx)
		logger.Debug("no local app config detected for machines deploy; fetching from backend ...")

		cfg, err = appconfig.FromRemoteApp(ctx, appNameFromContext)
		if err != nil {
			return nil, err
		}
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
