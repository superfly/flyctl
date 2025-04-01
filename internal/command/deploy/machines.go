package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/shlex"
	"github.com/logrusorgru/aurora"
	"github.com/morikuni/aec"
	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/command/deploy/statics"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/maps"
)

const (
	DefaultWaitTimeout            = 5 * time.Minute
	DefaultReleaseCommandTimeout  = 5 * time.Minute
	DefaultLeaseTtl               = 13 * time.Second
	DefaultMaxUnavailable         = 0.33
	DefaultVolumeInitialSizeGB    = 1
	DefaultGPUVolumeInitialSizeGB = 100
)

type MachineDeployment interface {
	DeployMachinesApp(context.Context) error
}

type MachineDeploymentArgs struct {
	AppCompact            *fly.AppCompact
	DeploymentImage       string
	Strategy              string
	EnvFromFlags          []string
	PrimaryRegionFlag     string
	SkipSmokeChecks       bool
	SkipHealthChecks      bool
	SkipDNSChecks         bool
	SkipReleaseCommand    bool
	MaxUnavailable        *float64
	RestartOnly           bool
	WaitTimeout           *time.Duration
	StopSignal            string
	LeaseTimeout          *time.Duration
	ReleaseCmdTimeout     *time.Duration
	Guest                 *fly.MachineGuest
	IncreasedAvailability bool
	AllocIP               string
	Org                   string
	UpdateOnly            bool
	Files                 []*fly.File
	ExcludeRegions        map[string]bool
	OnlyRegions           map[string]bool
	ExcludeMachines       map[string]bool
	OnlyMachines          map[string]bool
	ProcessGroups         map[string]bool
	MaxConcurrent         int
	VolumeInitialSize     int
	RestartPolicy         *fly.MachineRestartPolicy
	RestartMaxRetries     int
	DeployRetries         int
	BuildID               string
}

func argsFromManifest(manifest *DeployManifest, app *fly.AppCompact) MachineDeploymentArgs {
	return MachineDeploymentArgs{
		AppCompact:            app,
		DeploymentImage:       manifest.DeploymentImage,
		Strategy:              manifest.Strategy,
		EnvFromFlags:          manifest.EnvFromFlags,
		PrimaryRegionFlag:     manifest.PrimaryRegionFlag,
		SkipSmokeChecks:       manifest.SkipSmokeChecks,
		SkipHealthChecks:      manifest.SkipHealthChecks,
		SkipDNSChecks:         manifest.SkipDNSChecks,
		SkipReleaseCommand:    manifest.SkipReleaseCommand,
		MaxUnavailable:        manifest.MaxUnavailable,
		RestartOnly:           manifest.RestartOnly,
		WaitTimeout:           manifest.WaitTimeout,
		StopSignal:            manifest.StopSignal,
		LeaseTimeout:          manifest.LeaseTimeout,
		ReleaseCmdTimeout:     manifest.ReleaseCmdTimeout,
		Guest:                 manifest.Guest,
		IncreasedAvailability: manifest.IncreasedAvailability,
		UpdateOnly:            manifest.UpdateOnly,
		Files:                 manifest.Files,
		ExcludeRegions:        manifest.ExcludeRegions,
		OnlyRegions:           manifest.OnlyRegions,
		ExcludeMachines:       manifest.ExcludeMachines,
		OnlyMachines:          manifest.OnlyMachines,
		ProcessGroups:         manifest.ProcessGroups,
		MaxConcurrent:         manifest.MaxConcurrent,
		VolumeInitialSize:     manifest.VolumeInitialSize,
		RestartPolicy:         manifest.RestartPolicy,
		RestartMaxRetries:     manifest.RestartMaxRetries,
		DeployRetries:         manifest.DeployRetries,
	}
}

type machineDeployment struct {
	// apiClient is a client to use web.
	apiClient webClient
	// flapsClient is a client to use flaps.
	flapsClient flapsutil.FlapsClient
	io          *iostreams.IOStreams
	colorize    *iostreams.ColorScheme
	app         *fly.AppCompact
	appConfig   *appconfig.Config
	img         string
	// machineSet is this application's machines.
	machineSet            machine.MachineSet
	releaseCommandMachine machine.MachineSet
	volumes               map[string][]fly.Volume
	strategy              string
	releaseId             string
	releaseVersion        int
	skipSmokeChecks       bool
	skipHealthChecks      bool
	skipDNSChecks         bool
	skipReleaseCommand    bool
	maxUnavailable        float64
	restartOnly           bool
	waitTimeout           time.Duration
	stopSignal            string
	leaseTimeout          time.Duration
	leaseDelayBetween     time.Duration
	releaseCmdTimeout     time.Duration
	isFirstDeploy         bool
	machineGuest          *fly.MachineGuest
	increasedAvailability bool
	listenAddressChecked  sync.Map
	updateOnly            bool
	excludeRegions        map[string]bool
	onlyRegions           map[string]bool
	excludeMachines       map[string]bool
	onlyMachines          map[string]bool
	processGroups         map[string]bool
	maxConcurrent         int
	volumeInitialSize     int
	tigrisStatics         *statics.DeployerState
	deployRetries         int
	buildID               string
}

func NewMachineDeployment(ctx context.Context, args MachineDeploymentArgs) (_ MachineDeployment, err error) {
	var io = iostreams.FromContext(ctx)

	ctx, span := tracing.GetTracer().Start(ctx, "new_machines_deployment")
	defer span.End()

	if !args.RestartOnly && args.DeploymentImage == "" {
		return nil, fmt.Errorf("BUG: machines deployment created without specifying the image")
	}
	if args.RestartOnly && args.DeploymentImage != "" {
		return nil, fmt.Errorf("BUG: restartOnly machines deployment created and specified an image")
	}
	appConfig, err := determineAppConfigForMachines(ctx, args.EnvFromFlags, args.PrimaryRegionFlag, args.Strategy, args.MaxUnavailable, args.Files)
	if err != nil {
		tracing.RecordError(span, err, "failed to determine app config for machines")
		return nil, err
	}

	// TODO: Blend extraInfo into ValidationError and remove this hack
	if err, extraInfo := appConfig.ValidateGroups(ctx, lo.Keys(args.ProcessGroups)); err != nil {
		fmt.Fprint(io.ErrOut, extraInfo)
		tracing.RecordError(span, err, "failed to validate process groups")
		return nil, err
	}

	if args.AppCompact == nil {
		return nil, fmt.Errorf("BUG: args.AppCompact should be set when calling this method")
	}

	flapsClient := flapsutil.ClientFromContext(ctx)
	if flapsClient == nil {
		flapsClient, err = flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
			AppCompact: args.AppCompact,
			AppName:    args.AppCompact.Name,
		})
		if err != nil {
			tracing.RecordError(span, err, "failed to init flaps client")
			return nil, err
		}
	}

	if appConfig.Deploy != nil && appConfig.Deploy.ReleaseCommand != "" {
		_, err = shlex.Split(appConfig.Deploy.ReleaseCommand)
		if err != nil {
			tracing.RecordError(span, err, "failed to split release command")
			return nil, err
		}
	}

	var waitTimeout time.Duration
	switch {
	case args.WaitTimeout != nil:
		waitTimeout = *args.WaitTimeout
	case appConfig.Deploy != nil && appConfig.Deploy.WaitTimeout != nil:
		waitTimeout = appConfig.Deploy.WaitTimeout.Duration
	default:
		waitTimeout = DefaultWaitTimeout
	}

	var releaseCmdTimeout time.Duration
	switch {
	case args.ReleaseCmdTimeout != nil:
		releaseCmdTimeout = *args.ReleaseCmdTimeout
	case appConfig.Deploy != nil && appConfig.Deploy.ReleaseCommandTimeout != nil:
		releaseCmdTimeout = appConfig.Deploy.ReleaseCommandTimeout.Duration
	default:
		releaseCmdTimeout = DefaultReleaseCommandTimeout
	}

	var leaseTimeout time.Duration
	switch {
	case args.LeaseTimeout != nil:
		leaseTimeout = *args.LeaseTimeout
	default:
		leaseTimeout = DefaultLeaseTtl
	}

	leaseDelayBetween := (leaseTimeout - 1*time.Second) / 3
	if waitTimeout != DefaultWaitTimeout || leaseTimeout != DefaultLeaseTtl {
		terminal.Infof("Using wait timeout: %s lease timeout: %s delay between lease refreshes: %s\n", waitTimeout, leaseTimeout, leaseDelayBetween)
	}

	apiClient := flyutil.ClientFromContext(ctx)

	maxUnavailable := DefaultMaxUnavailable
	if appConfig.Deploy != nil && appConfig.Deploy.MaxUnavailable != nil {
		maxUnavailable = *appConfig.Deploy.MaxUnavailable
	}

	maxConcurrent := args.MaxConcurrent
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}

	md := &machineDeployment{
		apiClient:             apiClient,
		flapsClient:           flapsClient,
		io:                    io,
		colorize:              io.ColorScheme(),
		app:                   args.AppCompact,
		appConfig:             appConfig,
		img:                   args.DeploymentImage,
		skipSmokeChecks:       args.SkipSmokeChecks,
		skipHealthChecks:      args.SkipHealthChecks,
		skipDNSChecks:         args.SkipDNSChecks,
		skipReleaseCommand:    args.SkipReleaseCommand,
		restartOnly:           args.RestartOnly,
		maxUnavailable:        maxUnavailable,
		waitTimeout:           waitTimeout,
		stopSignal:            args.StopSignal,
		leaseTimeout:          leaseTimeout,
		leaseDelayBetween:     leaseDelayBetween,
		releaseCmdTimeout:     releaseCmdTimeout,
		increasedAvailability: args.IncreasedAvailability,
		updateOnly:            args.UpdateOnly,
		machineGuest:          args.Guest,
		excludeRegions:        args.ExcludeRegions,
		onlyRegions:           args.OnlyRegions,
		excludeMachines:       args.ExcludeMachines,
		onlyMachines:          args.OnlyMachines,
		maxConcurrent:         maxConcurrent,
		volumeInitialSize:     args.VolumeInitialSize,
		processGroups:         args.ProcessGroups,
		deployRetries:         args.DeployRetries,
		buildID:               args.BuildID,
	}
	if err := md.setStrategy(); err != nil {
		tracing.RecordError(span, err, "failed to set strategy")
		return nil, err
	}

	if err := md.setMachinesForDeployment(ctx); err != nil {
		tracing.RecordError(span, err, "failed to set machines for first deployemt")
		return nil, err
	}
	if err := md.setVolumes(ctx); err != nil {
		tracing.RecordError(span, err, "failed to set volumes")
		return nil, err
	}
	if err := md.setImg(ctx); err != nil {
		tracing.RecordError(span, err, "failed to set img")
		return nil, err
	}
	if err := md.setFirstDeploy(ctx); err != nil {
		tracing.RecordError(span, err, "failed to set first depoyment")
		return nil, err
	}

	// Provisioning must come after setVolumes
	if err := md.provisionFirstDeploy(ctx, args.AllocIP, args.Org); err != nil {
		tracing.RecordError(span, err, "failed to provision first depoloy")
		return nil, err
	}

	// validations must happen after every else
	if err := md.validateVolumeConfig(); err != nil {
		tracing.RecordError(span, err, "failed to validate volume config")
		return nil, err
	}
	if err = md.createReleaseInBackend(ctx); err != nil {
		tracing.RecordError(span, err, "failed to create release in backend")
		return nil, err
	}

	span.SetAttributes(md.ToSpanAttributes()...)
	return md, nil
}

func (md *machineDeployment) setFirstDeploy(_ context.Context) error {
	// Due to https://github.com/superfly/web/issues/1397 we have to be extra careful
	// by checking for any existent machine.
	// This is not exaustive as the app could still be scaled down to zero but the
	// workaround works better for now until it is fixed
	md.isFirstDeploy = md.isFirstDeploy || (!md.app.Deployed && md.machineSet.IsEmpty())
	return nil
}

func (md *machineDeployment) setMachinesForDeployment(ctx context.Context) error {
	ctx, span := tracing.GetTracer().Start(ctx, "set_machines_for_deployment")
	defer span.End()

	machines, releaseCmdMachine, err := md.flapsClient.ListFlyAppsMachines(ctx)
	if err != nil {
		tracing.RecordError(span, err, "failed to list machines")
		return err
	}

	nMachines := len(machines)
	if nMachines == 0 {
		terminal.Debug("Found no machines that are part of Fly Apps Platform. Checking for active machines...")
		activeMachines, err := md.flapsClient.ListActive(ctx)
		if err != nil {
			tracing.RecordError(span, err, "failed to list machines")
			return err
		}
		if len(activeMachines) > 0 {
			fmt.Fprintf(md.io.ErrOut, "%s Your app doesn't have any Fly Launch machines, so we'll create one now. Learn more at \nhttps://fly.io/docs/launch/\n\n", aurora.Yellow("[WARNING]"))
			md.isFirstDeploy = true
		}
	}

	filtersApplied := map[string]struct{}{}
	machines = slices.DeleteFunc(machines, func(m *fly.Machine) bool {
		if len(md.onlyRegions) > 0 {
			filtersApplied["--regions"] = struct{}{}

			if !md.onlyRegions[m.Region] {
				return true
			}
		}

		if len(md.excludeRegions) > 0 {
			filtersApplied["--exclude-regions"] = struct{}{}

			if md.excludeRegions[m.Region] {
				return true
			}
		}

		if len(md.onlyMachines) > 0 {
			filtersApplied["--only-machines"] = struct{}{}

			if !md.onlyMachines[m.ID] {
				return true
			}
		}

		if len(md.excludeMachines) > 0 {
			filtersApplied["--exclude-machines"] = struct{}{}

			if md.excludeMachines[m.ID] {
				return true
			}
		}

		if len(md.processGroups) > 0 {
			filtersApplied["--process-groups"] = struct{}{}

			if !md.processGroups[m.ProcessGroup()] {
				return true
			}
		}

		return false
	})

	if len(filtersApplied) > 0 {
		s := ""
		if len(filtersApplied) > 1 {
			s = "s"
		}

		filtersAppliedStr := strings.Join(maps.Keys(filtersApplied), "/")

		fmt.Fprintf(md.io.ErrOut, "%s filter%s applied, deploying to %d/%d machines\n", filtersAppliedStr, s, len(machines), nMachines)
	}

	for _, m := range machines {
		if m.Config != nil && m.Config.Metadata != nil {
			m.Config.Metadata[fly.MachineConfigMetadataKeyFlyctlVersion] = buildinfo.Version().String()
			if m.Config.Metadata[fly.MachineConfigMetadataKeyFlyProcessGroup] == "" {
				m.Config.Metadata[fly.MachineConfigMetadataKeyFlyProcessGroup] = md.appConfig.DefaultProcessName()
			}
		}
	}

	md.machineSet = machine.NewMachineSet(md.flapsClient, md.io, machines, true)
	var releaseCmdSet []*fly.Machine
	if releaseCmdMachine != nil {
		releaseCmdSet = []*fly.Machine{releaseCmdMachine}
	}
	md.releaseCommandMachine = machine.NewMachineSet(md.flapsClient, md.io, releaseCmdSet, true)
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

	unattached := lo.Filter(volumes, func(v fly.Volume, _ int) bool {
		return v.AttachedAllocation == nil && v.AttachedMachine == nil && v.HostStatus == "ok"
	})

	md.volumes = lo.GroupBy(unattached, func(v fly.Volume) string {
		return v.Name
	})
	return nil
}

func (md *machineDeployment) popVolumeFor(name, region string) *fly.Volume {
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
		lo.Map(md.machineSet.GetMachines(), func(lm machine.LeasableMachine, _ int) *fly.Machine {
			return lm.Machine()
		}),
		func(m *fly.Machine) string {
			return m.ProcessGroup()
		})

	for _, groupName := range md.ProcessNames() {
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
				mConfig := m.GetConfig()
				if mntDst == "" && len(mConfig.Mounts) != 0 {
					// TODO: Detaching a volume from a machine is possible, but it usually means a missconfiguration.
					// We should show a warning and ask the user for confirmation and let it happen instead of failing here.
					return fmt.Errorf(
						"machine %s [%s] has a volume mounted but app config does not specify a volume; "+
							"remove the volume from the machine or add a [mounts] section to fly.toml",
						m.ID, groupName,
					)
				}

				if mntDst != "" && len(mConfig.Mounts) == 0 {
					// Attaching a volume to an existing machine is not possible, but we replace the machine
					// by another running on the same zone than the volume.
					needsVol[mntSrc] = append(needsVol[mntSrc], m.Region)
				}

				if mms := mConfig.Mounts; len(mms) > 0 && mntSrc != "" && mms[0].Name != "" && mntSrc != mms[0].Name {
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
				currentPerRegion := lo.CountValuesBy(md.volumes[volSrc], func(v fly.Volume) string { return v.Region })
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
						"Process group '%s' needs volumes with name '%s' to fulfill mounts defined in fly.toml; "+
							"Run `fly volume create %s -r REGION -n COUNT` for the following regions and counts: %s",
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
	latestImg, err := md.apiClient.LatestImage(ctx, md.app.Name)
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

func (md *machineDeployment) setStrategy() error {
	md.strategy = "rolling"
	if md.appConfig.Deploy != nil && md.appConfig.Deploy.Strategy != "" {
		md.strategy = md.appConfig.Deploy.Strategy
	}
	return nil
}

func (md *machineDeployment) createReleaseInBackend(ctx context.Context) error {
	ctx, span := tracing.GetTracer().Start(ctx, "create_backend_release")
	defer span.End()

	resp, err := md.apiClient.CreateRelease(ctx, fly.CreateReleaseInput{
		AppId:           md.app.Name,
		PlatformVersion: "machines",
		Strategy:        fly.DeploymentStrategy(strings.ToUpper(md.strategy)),
		Definition:      md.appConfig,
		Image:           md.img,
		BuildId:         md.buildID,
	})
	if err != nil {
		tracing.RecordError(span, err, "failed to create machine release")
		return err
	}
	md.releaseId = resp.CreateRelease.Release.Id
	md.releaseVersion = resp.CreateRelease.Release.Version
	return nil
}

func (md *machineDeployment) updateReleaseInBackend(ctx context.Context, status string, metadata *fly.ReleaseMetadata) error {
	ctx, span := tracing.GetTracer().Start(ctx, "update_release_in_backend", trace.WithAttributes(
		attribute.String("release_id", md.releaseId),
		attribute.String("status", status),
	))
	defer span.End()

	input := fly.UpdateReleaseInput{
		ReleaseId: md.releaseId,
		Status:    status,
		Metadata:  metadata,
	}

	_, err := md.apiClient.UpdateRelease(ctx, input)

	if err != nil {
		tracing.RecordError(span, err, "failed to update machine release")
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

func determineAppConfigForMachines(ctx context.Context, envFromFlags []string, primaryRegion, strategy string, maxUnavailable *float64, files []*fly.File) (*appconfig.Config, error) {
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

func (md *machineDeployment) ProcessNames() (names []string) {
	names = md.appConfig.ProcessNames()
	if len(md.processGroups) > 0 {
		names = slices.DeleteFunc(names, func(name string) bool {
			return !md.processGroups[name]
		})
	}
	return
}

func (md *machineDeployment) ToSpanAttributes() []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("deployment.app.name", md.app.Name),
		attribute.String("deployment.image", md.img),
		attribute.String("deployment.strategy", md.strategy),
		attribute.Bool("deployment.skip_smoke_checks", md.skipSmokeChecks),
		attribute.Bool("deployment.skip_health_checks", md.skipHealthChecks),
		attribute.Bool("deployment.skip_dns_checks", md.skipDNSChecks),
		attribute.Bool("deployment.restart_only", md.restartOnly),
		attribute.Float64("deployment.max_unavailable", md.maxUnavailable),
		attribute.Float64("deployment.wait_timeout", md.waitTimeout.Seconds()),
		attribute.Float64("deployment.lease_timeout", md.leaseTimeout.Seconds()),
		attribute.Float64("deployment.lease_delay_between", md.leaseDelayBetween.Seconds()),
		attribute.Float64("deployment.release_cmd_timeout", md.releaseCmdTimeout.Seconds()),
		attribute.Bool("deployment.increased_availability", md.increasedAvailability),
		attribute.Bool("deployment.update_only", md.updateOnly),
		attribute.Int("deployment.max_concurrency", md.maxConcurrent),
		attribute.Int("deployment.volume_initial_size", md.volumeInitialSize),
	}

	b, err := json.Marshal(md.excludeRegions)
	if err == nil {
		attrs = append(attrs, attribute.String("deployment.exclude_regions", string(b)))
	}

	b, err = json.Marshal(md.onlyRegions)
	if err == nil {
		attrs = append(attrs, attribute.String("deployment.only_regions", string(b)))
	}

	b, err = json.Marshal(md.processGroups)
	if err == nil {
		attrs = append(attrs, attribute.String("deployment.process_groups", string(b)))
	}

	return attrs
}
