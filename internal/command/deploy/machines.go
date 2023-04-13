package deploy

import (
	"context"
	"fmt"
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
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
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
	NewVolumeName     string
}

type machineDeployment struct {
	apiClient             *api.Client
	gqlClient             graphql.Client
	flapsClient           *flaps.Client
	io                    *iostreams.IOStreams
	colorize              *iostreams.ColorScheme
	app                   *api.AppCompact
	appConfig             *appconfig.Config
	img                   string
	machineSet            machine.MachineSet
	releaseCommandMachine machine.MachineSet
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
	if args.NewVolumeName != "" && appConfig.Mounts != nil {
		appConfig.Mounts.Source = args.NewVolumeName
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

	volumes, err := md.apiClient.GetVolumes(ctx, md.app.Name)
	if err != nil {
		return fmt.Errorf("Error fetching application volumes: %w", err)
	}

	md.volumes = lo.Filter(volumes, func(v api.Volume, _ int) bool {
		return v.Name == md.appConfig.Mounts.Source && v.AttachedAllocation == nil && v.AttachedMachine == nil
	})
	return nil
}

func (md *machineDeployment) validateVolumeConfig() error {
	volumeDestination := ""
	if md.appConfig.Mounts != nil {
		volumeDestination = md.appConfig.Mounts.Destination
	}

	for _, m := range md.machineSet.GetMachines() {
		mid := m.Machine().ID
		mountsConfig := m.Machine().Config.Mounts
		if len(mountsConfig) > 1 {
			return fmt.Errorf("error machine %s has %d mounts and expected 1", mid, len(mountsConfig))
		}
		if volumeDestination == "" && len(mountsConfig) != 0 {
			return fmt.Errorf("error machine %s has a volume mounted and app config does not specify a volume; remove the volume from the machine or add a [mounts] configuration to fly.toml", mid)
		}
		if volumeDestination != "" && len(mountsConfig) == 0 {
			return fmt.Errorf("error machine %s does not have a volume configured and fly.toml expects one with destination %s; remove the [mounts] configuration in fly.toml or use the machines API to add a volume to this machine", mid, volumeDestination)
		}
	}

	if md.machineSet.IsEmpty() && volumeDestination != "" && len(md.volumes) == 0 {
		return fmt.Errorf("error new machine requires an unattached volume named '%s' on mount destination '%s'",
			md.appConfig.Mounts.Source, volumeDestination)
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

func (md *machineDeployment) provisionIpsOnFirstDeploy(ctx context.Context) error {
	// Provision only if the app hasn't been deployed and have defined services
	if md.app.Deployed || !md.machineSet.IsEmpty() || len(md.appConfig.AllServices()) == 0 {
		return nil
	}

	// Do not touch IPs if there are already allocated
	ipAddrs, err := md.apiClient.GetIPAddresses(ctx, md.app.Name)
	if err != nil {
		return fmt.Errorf("error detecting ip addresses allocated to %s app: %w", md.app.Name, err)
	}
	if len(ipAddrs) > 0 {
		return nil
	}

	switch md.appConfig.HasNonHttpAndHttpsStandardServices() {
	case true:
		hasUdpService := md.appConfig.HasUdpService()

		ipStuffStr := "a dedicated ipv4 address"
		if !hasUdpService {
			ipStuffStr = "dedicated ipv4 and ipv6 addresses"
		}

		confirmDedicatedIp, err := prompt.Confirmf(ctx, "Would you like to allocate %s now?", ipStuffStr)
		if confirmDedicatedIp && err == nil {
			v4Dedicated, err := md.apiClient.AllocateIPAddress(ctx, md.app.Name, "v4", "", nil, "")
			if err != nil {
				return err
			}
			fmt.Fprintf(md.io.Out, "Allocated dedicated ipv4: %s\n", v4Dedicated.Address)

			if !hasUdpService {
				v6Dedicated, err := md.apiClient.AllocateIPAddress(ctx, md.app.Name, "v6", "", nil, "")
				if err != nil {
					return err
				}
				fmt.Fprintf(md.io.Out, "Allocated dedicated ipv6: %s\n", v6Dedicated.Address)
			}
		}

	case false:
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
