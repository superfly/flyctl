package deploy

import (
	"context"
	"fmt"
	"strings"
	"sync"
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
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
)

const (
	DefaultWaitTimeout           = 2 * time.Minute
	DefaultReleaseCommandTimeout = 5 * time.Minute
	DefaultLeaseTtl              = 13 * time.Second
	DefaultMaxUnavailable        = 0.33
)

type MachineDeployment interface {
	DeployMachinesApp(context.Context) error
}

type MachineDeploymentArgs struct {
	AppCompact             *api.AppCompact
	DeploymentImage        string
	Strategy               string
	EnvFromFlags           []string
	PrimaryRegionFlag      string
	SkipSmokeChecks        bool
	SkipHealthChecks       bool
	MaxUnavailable         *float64
	RestartOnly            bool
	WaitTimeout            time.Duration
	LeaseTimeout           time.Duration
	ReleaseCmdTimeout      time.Duration
	Guest                  *api.MachineGuest
	IncreasedAvailability  bool
	AllocPublicIP          bool
	UpdateOnly             bool
	Files                  []*api.File
	ExcludeRegions         map[string]interface{}
	OnlyRegions            map[string]interface{}
	ImmediateMaxConcurrent int
	VolumeInitialSize      int
}

type machineDeployment struct {
	apiClient              *api.Client
	gqlClient              graphql.Client
	flapsClient            *flaps.Client
	io                     *iostreams.IOStreams
	colorize               *iostreams.ColorScheme
	app                    *api.AppCompact
	appConfig              *appconfig.Config
	img                    string
	machineSet             machine.MachineSet
	releaseCommandMachine  machine.MachineSet
	volumes                map[string][]api.Volume
	strategy               string
	releaseId              string
	releaseVersion         int
	skipSmokeChecks        bool
	skipHealthChecks       bool
	maxUnavailable         float64
	restartOnly            bool
	waitTimeout            time.Duration
	leaseTimeout           time.Duration
	leaseDelayBetween      time.Duration
	releaseCmdTimeout      time.Duration
	isFirstDeploy          bool
	machineGuest           *api.MachineGuest
	increasedAvailability  bool
	listenAddressChecked   sync.Map
	updateOnly             bool
	excludeRegions         map[string]interface{}
	onlyRegions            map[string]interface{}
	immediateMaxConcurrent int
	volumeInitialSize      int
}

func NewMachineDeployment(ctx context.Context, args MachineDeploymentArgs) (MachineDeployment, error) {
	if !args.RestartOnly && args.DeploymentImage == "" {
		return nil, fmt.Errorf("BUG: machines deployment created without specifying the image")
	}
	if args.RestartOnly && args.DeploymentImage != "" {
		return nil, fmt.Errorf("BUG: restartOnly machines deployment created and specified an image")
	}
	appConfig, err := determineAppConfigForMachines(ctx, args.EnvFromFlags, args.PrimaryRegionFlag, args.Strategy, args.MaxUnavailable, args.Files)
	if err != nil {
		return nil, err
	}

	// TODO: Blend extraInfo into ValidationError and remove this hack
	if err, extraInfo := appConfig.Validate(ctx); err != nil {
		fmt.Fprintf(iostreams.FromContext(ctx).ErrOut, extraInfo)
		return nil, err
	}

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
	io := iostreams.FromContext(ctx)
	apiClient := client.FromContext(ctx).API()

	maxUnavailable := DefaultMaxUnavailable
	if appConfig.Deploy != nil && appConfig.Deploy.MaxUnavailable != nil {
		maxUnavailable = *appConfig.Deploy.MaxUnavailable
	}

	immedateMaxConcurrent := args.ImmediateMaxConcurrent
	if immedateMaxConcurrent < 1 {
		immedateMaxConcurrent = 1
	}
	volumeInitialSize := 1
	if args.VolumeInitialSize > 0 {
		volumeInitialSize = args.VolumeInitialSize
	}

	md := &machineDeployment{
		apiClient:              apiClient,
		gqlClient:              apiClient.GenqClient,
		flapsClient:            flapsClient,
		io:                     io,
		colorize:               io.ColorScheme(),
		app:                    args.AppCompact,
		appConfig:              appConfig,
		img:                    args.DeploymentImage,
		skipSmokeChecks:        args.SkipSmokeChecks,
		skipHealthChecks:       args.SkipHealthChecks,
		restartOnly:            args.RestartOnly,
		maxUnavailable:         maxUnavailable,
		waitTimeout:            waitTimeout,
		leaseTimeout:           leaseTimeout,
		leaseDelayBetween:      leaseDelayBetween,
		releaseCmdTimeout:      args.ReleaseCmdTimeout,
		increasedAvailability:  args.IncreasedAvailability,
		updateOnly:             args.UpdateOnly,
		machineGuest:           args.Guest,
		excludeRegions:         args.ExcludeRegions,
		onlyRegions:            args.OnlyRegions,
		immediateMaxConcurrent: immedateMaxConcurrent,
		volumeInitialSize:      volumeInitialSize,
	}
	if err := md.setStrategy(); err != nil {
		return nil, err
	}
	if err := md.setMachinesForDeployment(ctx); err != nil {
		return nil, err
	}
	if err := md.setVolumes(ctx); err != nil {
		return nil, err
	}
	if err := md.setImg(ctx); err != nil {
		return nil, err
	}
	if err := md.setFirstDeploy(ctx); err != nil {
		return nil, err
	}

	// Provisioning must come after setVolumes
	if err := md.provisionFirstDeploy(ctx, args.AllocPublicIP); err != nil {
		return nil, err
	}

	// validations must happen after every else
	if err := md.validateVolumeConfig(); err != nil {
		return nil, err
	}
	if err = md.createReleaseInBackend(ctx); err != nil {
		return nil, err
	}
	return md, nil
}

func (md *machineDeployment) setFirstDeploy(ctx context.Context) error {
	// Due to https://github.com/superfly/web/issues/1397 we have to be extra careful
	// by checking for any existent machine.
	// This is not exaustive as the app could still be scaled down to zero but the
	// workaround works better for now until it is fixed
	md.isFirstDeploy = !md.app.Deployed && md.machineSet.IsEmpty()
	return nil
}

func (md *machineDeployment) setMachinesForDeployment(ctx context.Context) error {
	machines, releaseCmdMachine, err := md.flapsClient.ListFlyAppsMachines(ctx)
	if err != nil {
		return err
	}

	if len(machines) == 0 {
		terminal.Debug("Found no machines that are part of Fly Apps Platform. Checking for active machines...")
		activeMachines, err := md.flapsClient.ListActive(ctx)
		if err != nil {
			return err
		}
		if len(activeMachines) > 0 {
			return fmt.Errorf(
				"found %d machines that are unmanaged. `fly deploy` only updates machines with %s=%s in their metadata. Use `fly machine list` to list machines and `fly machine update --metadata %s=%s <machine id>` to update individual machines with the metadata. Once done, `fly deploy` will update machines with the metadata based on your %s app configuration",
				len(activeMachines),
				api.MachineConfigMetadataKeyFlyPlatformVersion,
				api.MachineFlyPlatformVersion2,
				api.MachineConfigMetadataKeyFlyPlatformVersion,
				api.MachineFlyPlatformVersion2,
				appconfig.DefaultConfigFileName,
			)
		}
	}

	if len(md.onlyRegions) > 0 {
		var onlyRegionMachines []*api.Machine
		for _, m := range machines {
			if _, present := md.onlyRegions[m.Region]; present {
				onlyRegionMachines = append(onlyRegionMachines, m)
			}
		}
		fmt.Fprintf(md.io.ErrOut, "--only-regions filter applied, deploying to %d/%d machines\n", len(onlyRegionMachines), len(machines))
		machines = onlyRegionMachines
	}
	if len(md.excludeRegions) > 0 {
		var excludeRegionMachines []*api.Machine
		for _, m := range machines {
			if _, present := md.excludeRegions[m.Region]; !present {
				excludeRegionMachines = append(excludeRegionMachines, m)
			}
		}
		fmt.Fprintf(md.io.ErrOut, "--exclude-regions filter applied, deploying to %d/%d machines\n", len(excludeRegionMachines), len(machines))
		machines = excludeRegionMachines
	}

	for _, m := range machines {
		if m.Config != nil && m.Config.Metadata != nil {
			m.Config.Metadata[api.MachineConfigMetadataKeyFlyctlVersion] = buildinfo.Version().String()
			if m.Config.Metadata[api.MachineConfigMetadataKeyFlyProcessGroup] == "" {
				m.Config.Metadata[api.MachineConfigMetadataKeyFlyProcessGroup] = md.appConfig.DefaultProcessName()
			}
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

func (md *machineDeployment) setVolumes(ctx context.Context) error {
	if len(md.appConfig.Mounts) == 0 {
		return nil
	}

	volumes, err := md.flapsClient.GetVolumes(ctx)
	if err != nil {
		return fmt.Errorf("Error fetching application volumes: %w", err)
	}

	unattached := lo.Filter(volumes, func(v api.Volume, _ int) bool {
		return v.AttachedAllocation == nil && v.AttachedMachine == nil
	})

	md.volumes = lo.GroupBy(unattached, func(v api.Volume) string {
		return v.Name
	})
	return nil
}

func (md *machineDeployment) popVolumeFor(name, region string) *api.Volume {
	volumes := md.volumes[name]
	for idx, v := range volumes {
		if region == "" || region == v.Region {
			md.volumes[name] = append(volumes[:idx], volumes[idx+1:]...)
			return &v
		}
	}
	return nil
}

func (md *machineDeployment) validateVolumeConfig() error {
	machineGroups := lo.GroupBy(
		lo.Map(md.machineSet.GetMachines(), func(lm machine.LeasableMachine, _ int) *api.Machine {
			return lm.Machine()
		}),
		func(m *api.Machine) string {
			return m.ProcessGroup()
		})

	for _, groupName := range md.appConfig.ProcessNames() {
		groupConfig, err := md.appConfig.Flatten(groupName)
		if err != nil {
			return err
		}

		switch ms := machineGroups[groupName]; len(ms) > 0 {
		case true:
			// For groups with machines, check the attached volumes match expected mounts
			var mntSrc, mntDst string
			if len(groupConfig.Mounts) > 0 {
				mntSrc = groupConfig.Mounts[0].Source
				mntDst = groupConfig.Mounts[0].Destination
			}

			needsVol := map[string][]string{}

			for _, m := range ms {
				if mntDst == "" && len(m.Config.Mounts) != 0 {
					// TODO: Detaching a volume from a machine is possible, but it usually means a missconfiguration.
					// We should show a warning and ask the user for confirmation and let it happen instead of failing here.
					return fmt.Errorf(
						"machine %s [%s] has a volume mounted but app config does not specify a volume; "+
							"remove the volume from the machine or add a [mounts] section to fly.toml",
						m.ID, groupName,
					)
				}

				if mntDst != "" && len(m.Config.Mounts) == 0 {
					// Attaching a volume to an existing machine is not possible, but we replace the machine
					// by another running on the same zone than the volume.
					needsVol[mntSrc] = append(needsVol[mntSrc], m.Region)
				}

				if mms := m.Config.Mounts; len(mms) > 0 && mntSrc != "" && mms[0].Name != "" && mntSrc != mms[0].Name {
					// TODO: Changed the attached volume to an existing machine is not possible, but it could replace the machine
					// by another running on the same zone than the new volume.
					return fmt.Errorf(
						"machine %s [%s] can't update the attached volume %s with name '%s' by '%s'",
						m.ID, groupName, mntSrc, mms[0].Volume, mms[0].Name,
					)
				}
			}

			// Compute the volume differences per region
			for volSrc, regions := range needsVol {
				currentPerRegion := lo.CountValuesBy(md.volumes[volSrc], func(v api.Volume) string { return v.Region })
				needsPerRegion := lo.CountValues(regions)

				var missing []string
				for rn, rc := range needsPerRegion {
					diff := rc - currentPerRegion[rn]
					if diff > 0 {
						missing = append(missing, fmt.Sprintf("%s=%d", rn, diff))
					}
				}
				if len(missing) > 0 {
					// TODO: May change this by a prompt to create new volumes right away (?)
					return fmt.Errorf(
						"Process group '%s' needs volumes with name '%s' to fullfill mounts defined in fly.toml; "+
							"Run `fly volume create %s -r REGION` for the following regions and counts: %s",
						groupName, volSrc, volSrc, strings.Join(missing, " "),
					)
				}
			}

		case false:
			// Check if there are unattached volumes for new groups with mounts
			for _, m := range groupConfig.Mounts {
				if vs := md.volumes[m.Source]; len(vs) == 0 {
					return fmt.Errorf(
						"creating a new machine in group '%s' requires an unattached '%s' volume. Create it with `fly volume create %s`",
						groupName, m.Source, m.Source)
				}
			}
		}
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

func (md *machineDeployment) setStrategy() error {
	md.strategy = "rolling"
	if md.appConfig.Deploy != nil && md.appConfig.Deploy.Strategy != "" {
		md.strategy = md.appConfig.Deploy.Strategy
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
		AppId:           md.app.Name,
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

func (md *machineDeployment) updateReleaseInBackend(ctx context.Context, status string) error {
	_ = `# @genqlient
	mutation MachinesUpdateRelease($input:UpdateReleaseInput!) {
		updateRelease(input:$input) {
			release {
				id
			}
		}
	}
	`
	input := gql.UpdateReleaseInput{
		ReleaseId: md.releaseId,
		Status:    status,
	}
	_, err := gql.MachinesUpdateRelease(ctx, md.gqlClient, input)
	if err != nil {
		return err
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

func determineAppConfigForMachines(ctx context.Context, envFromFlags []string, primaryRegion, strategy string, maxUnavailable *float64, files []*api.File) (*appconfig.Config, error) {
	appConfig := appconfig.ConfigFromContext(ctx)
	if appConfig == nil {
		return nil, fmt.Errorf("BUG: application configuration must come in the context, be sure to pass it before calling NewMachineDeployment")
	}

	if len(envFromFlags) > 0 {
		var parsedEnv map[string]string
		parsedEnv, err := cmdutil.ParseKVStringsToMap(envFromFlags)
		if err != nil {
			return nil, fmt.Errorf("failed parsing environment: %w", err)
		}
		appConfig.SetEnvVariables(parsedEnv)
	}

	if strategy != "" {
		if appConfig.Deploy == nil {
			appConfig.Deploy = &appconfig.Deploy{}
		}
		appConfig.Deploy.Strategy = strategy
	}
	if maxUnavailable != nil {
		if appConfig.Deploy == nil {
			appConfig.Deploy = &appconfig.Deploy{}
		}
		appConfig.Deploy.MaxUnavailable = maxUnavailable
	}

	// deleting this block will result in machines not being deployed in the user selected region
	if primaryRegion != "" {
		appConfig.PrimaryRegion = primaryRegion
	}

	// Always prefer the app name passed via --app
	appName := appconfig.NameFromContext(ctx)
	if appName != "" {
		appConfig.AppName = appName
	}

	// Merge in any files passed via --file flags.
	if err := appConfig.MergeFiles(files); err != nil {
		return nil, err
	}

	return appConfig, nil
}
