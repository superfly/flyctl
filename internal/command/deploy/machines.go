package deploy

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/spinner"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
)

type MachineDeployment interface {
	DeployMachinesApp(context.Context) error
}

type MachineDeploymentArgs struct {
	Strategy             string
	AutoConfirmMigration bool
	RestartOnly          bool
}

type machineDeployment struct {
	gqlClient                  graphql.Client
	flapsClient                *flaps.Client
	app                        *api.AppCompact
	appConfig                  *app.Config
	img                        *imgsrc.DeploymentImage
	strategy                   string
	releaseId                  string
	releaseVersion             int
	autoConfirmAppsV2Migration bool
	restartOnly                bool
}

func NewMachineDeployment(ctx context.Context, args MachineDeploymentArgs) (MachineDeployment, error) {
	appConfig, err := determineAppConfig(ctx)
	if err != nil {
		return nil, err
	}
	err = appConfig.Validate()
	if err != nil {
		return nil, err
	}
	app, err := client.FromContext(ctx).API().GetAppCompact(ctx, appConfig.AppName)
	if err != nil {
		return nil, err
	}
	img, err := determineImage(ctx, appConfig)
	if err != nil {
		return nil, err
	}
	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return nil, err
	}
	md := &machineDeployment{
		gqlClient:                  client.FromContext(ctx).API().GenqClient,
		flapsClient:                flapsClient,
		app:                        app,
		appConfig:                  appConfig,
		img:                        img,
		autoConfirmAppsV2Migration: args.AutoConfirmMigration,
		restartOnly:                args.RestartOnly,
	}
	md.setStrategy(args.Strategy)
	err = md.createReleaseInBackend(ctx)
	if err != nil {
		return nil, err
	}
	return md, nil
}

func (md *machineDeployment) runReleaseCommand(ctx context.Context) (err error) {
	if md.restartOnly {
		return nil
	}
	if md.appConfig.Deploy == nil || md.appConfig.Deploy.ReleaseCommand == "" {
		return nil
	}

	io := iostreams.FromContext(ctx)

	msg := fmt.Sprintf("Running release command: %s", md.appConfig.Deploy.ReleaseCommand)
	spin := spinner.Run(io, msg)
	defer spin.StopWithSuccess()

	machineConf := md.baseMachineConfig()

	machineConf.Metadata["process_group"] = api.MachineProcessGroupReleaseCommand

	// Override the machine default command to run the release command
	machineConf.Init.Cmd = strings.Split(md.appConfig.Deploy.ReleaseCommand, " ")

	launchMachineInput := api.LaunchMachineInput{
		AppID:   md.app.ID,
		OrgSlug: md.app.Organization.ID,
		Config:  machineConf,
	}

	// Ensure release commands run in the primary region
	if md.appConfig.PrimaryRegion != "" {
		launchMachineInput.Region = md.appConfig.PrimaryRegion
	}

	// We don't want temporary release command VMs to serve traffic, so kill the services
	machineConf.Services = nil

	machine, err := md.flapsClient.Launch(ctx, launchMachineInput)
	if err != nil {
		return err
	}

	removeInput := api.RemoveMachineInput{
		AppID: md.app.Name,
		ID:    machine.ID,
	}

	// Make sure we clean up the release command VM
	defer md.flapsClient.Destroy(ctx, removeInput)

	// Ensure the command starts running
	err = md.flapsClient.Wait(ctx, machine, "started")

	if err != nil {
		return err
	}

	// Wait for the release command VM to stop before moving on
	err = md.flapsClient.Wait(ctx, machine, "stopped")

	if err != nil {
		return fmt.Errorf("failed determining whether the release command finished. %w", err)
	}

	var lastExitEvent *api.MachineEvent
	var pollMaxAttempts int = 10
	var pollAttempts int = 0

	// Poll until the 'stopped' event arrives, so we can determine the release command exit status
	for {
		if pollAttempts >= pollMaxAttempts {
			return fmt.Errorf("could not determine whether the release command succeeded, so aborting the deployment")
		}

		machine, err = md.flapsClient.Get(ctx, machine.ID)

		for _, event := range machine.Events {
			if event.Type != "exit" {
				continue
			}

			if lastExitEvent == nil || event.Timestamp > lastExitEvent.Timestamp {
				lastExitEvent = event
			}
		}

		if lastExitEvent != nil {
			break
		}

		time.Sleep(1 * time.Second)
		pollAttempts += 1
	}

	exitCode := lastExitEvent.Request.ExitEvent.ExitCode

	if exitCode != 0 {
		return fmt.Errorf("release command exited with non-zero status of %d", exitCode)
	}

	return
}

func (md *machineDeployment) DeployMachinesApp(ctx context.Context) (err error) {
	if err := md.runReleaseCommand(ctx); err != nil {
		return fmt.Errorf("release command failed - aborting deployment. %w", err)
	}

	io := iostreams.FromContext(ctx)
	ctx = flaps.NewContext(ctx, md.flapsClient)

	regionCode := md.appConfig.PrimaryRegion

	machineConfig := md.baseMachineConfig()
	machineConfig.Metadata["process_group"] = api.MachineProcessGroupApp
	machineConfig.Init.Cmd = nil

	launchInput := api.LaunchMachineInput{
		AppID:   md.app.Name,
		OrgSlug: md.app.Organization.ID,
		Config:  machineConfig,
		Region:  regionCode,
	}

	machines, err := md.flapsClient.ListFlyAppsMachines(ctx)
	if err != nil {
		return err
	}

	// migrate non-platform machines into fly platform
	if len(machines) == 0 {
		terminal.Debug("Found no machines that are part of Fly Apps platform. Check for other machines...")
		machines, err = md.flapsClient.ListActive(ctx)
		if err != nil {
			return err
		}
		if len(machines) > 0 {
			rows := [][]string{}
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
			terminal.Warnf("Found %d machines that are not part of Fly Apps platform:\n", len(machines))
			_ = render.Table(io.Out, md.app.Name, rows, "ID", "Name", "State", "Region", "Image", "IP Address", "Volume", "Created", "Last Updated")
			if !md.autoConfirmAppsV2Migration {
				switch confirmed, err := prompt.Confirmf(ctx, "Migrate %d existing machines into Fly Apps platform?", len(machines)); {
				case err == nil:
					if !confirmed {
						terminal.Info("Skipping machines migration to Fly Apps platforms and the deployment")
						return nil
					}
				case prompt.IsNonInteractive(err):
					return prompt.NonInteractiveError("not running interactively, use --auto-confirm flag to confirm")
				default:
					return err
				}
			}
			terminal.Infof("Migrating %d machines to the Fly Apps platform\n", len(machines))
		}
	}

	msg := fmt.Sprintf("Deploying with %s strategy", md.strategy)
	spin := spinner.Run(io, msg)
	defer spin.StopWithSuccess()

	if len(machines) > 0 {

		// FIXME: pull this out to its own method, and do retries for a little bit before giving up
		for _, machine := range machines {
			leaseTTL := api.IntPointer(30)
			lease, err := md.flapsClient.AcquireLease(ctx, machine.ID, leaseTTL)
			if err != nil {
				return err
			}
			machine.LeaseNonce = lease.Data.Nonce

			defer releaseLease(ctx, machine)
		}

		// FIXME: handle deploy strategy: rolling, immediate, canary, bluegreen

		for _, machine := range machines {
			launchInput.ID = machine.ID

			if md.restartOnly {
				launchInput.Config = machine.Config
			}

			launchInput.Region = machine.Region

			machineConfig.Metadata = machine.Config.Metadata

			if machineConfig.Metadata == nil {
				machineConfig.Metadata = map[string]string{
					"process_group": "app",
				}
			}

			if md.app.IsPostgresApp() {
				machineConfig.Metadata["fly-managed-postgres"] = "true"
			}

			if launchInput.Config.Env["PRIMARY_REGION"] == "" {
				if launchInput.Config.Env == nil {
					launchInput.Config.Env = map[string]string{}
				}
				launchInput.Config.Env["PRIMARY_REGION"] = machine.Config.Env["PRIMARY_REGION"]
			}

			launchInput.Config.Checks = machine.Config.Checks

			if machine.Config.Guest != nil {
				launchInput.Config.Guest = machine.Config.Guest
			}

			// Until mounts are supported in fly.toml, ensure deployments
			// maintain any existing volume attachments
			if machine.Config.Mounts != nil {
				launchInput.Config.Mounts = machine.Config.Mounts
			}

			updateResult, err := md.flapsClient.Update(ctx, launchInput, machine.LeaseNonce)
			if err != nil {
				if md.strategy != "immediate" {
					return err

				} else {
					fmt.Printf("Continuing after error: %s\n", err)
				}
			}

			if md.strategy != "immediate" {
				err = md.flapsClient.Wait(ctx, updateResult, "started")
				if err != nil {
					return err
				}
			}
		}

	} else {
		fmt.Fprintf(io.Out, "Launching VM with image %s\n", launchInput.Config.Image)
		_, err = md.flapsClient.Launch(ctx, launchInput)
		if err != nil {
			return err
		}
	}

	return
}

func releaseLease(ctx context.Context, machine *api.Machine) error {
	var client = flaps.FromContext(ctx)

	if err := client.ReleaseLease(ctx, machine.ID, machine.LeaseNonce); err != nil {
		return fmt.Errorf("failed to release lease: %w", err)
	}

	return nil
}

func (md *machineDeployment) setStrategy(passedInStrategy string) {
	if passedInStrategy != "" {
		md.strategy = passedInStrategy
		return
	}
	stratFromConfig := md.appConfig.GetDeployStrategy()
	if stratFromConfig != "" {
		md.strategy = stratFromConfig
		return
	}
	// FIXME: any other checks we want to do here? e.g., we used to do canary if any max_per_region==0 && app.distinct_regions?==false
	md.strategy = "rolling"
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
		Image:           md.img.Tag,
		PlatformVersion: "machines",
		Strategy:        gql.DeploymentStrategy(strings.ToUpper(md.strategy)),
		Definition:      md.appConfig.Definition,
	}
	resp, err := gql.MachinesCreateRelease(ctx, md.gqlClient, input)
	if err != nil {
		return err
	}
	md.releaseId = resp.CreateRelease.Release.Id
	md.releaseVersion = resp.CreateRelease.Release.Version
	return nil
}

func (md *machineDeployment) flyRelease() string {
	return fmt.Sprintf("%s_%d", md.releaseId, md.releaseVersion)
}

func (md *machineDeployment) baseMachineConfig() *api.MachineConfig {
	machineConfig := &api.MachineConfig{}
	machineConfig.Metadata = map[string]string{
		api.MachineConfigMetadataKeyFlyPlatformVersion: api.MachineFlyPlatformVersion2,
		api.MachineConfigMetadataKeyFlyRelease:         md.flyRelease(),
	}

	if md.restartOnly {
		return machineConfig
	}

	machineConfig.Image = md.img.Tag

	// Convert the new, slimmer http service config to standard services
	if md.appConfig.HttpService != nil {
		concurrency := md.appConfig.HttpService.Concurrency

		if concurrency != nil {
			if concurrency.Type == "" {
				concurrency.Type = "requests"
			}
			if concurrency.HardLimit == 0 {
				concurrency.HardLimit = 25
			}
			if concurrency.SoftLimit == 0 {
				concurrency.SoftLimit = int(math.Ceil(float64(concurrency.HardLimit) * 0.8))
			}
		}

		httpService := api.MachineService{
			Protocol:     "tcp",
			InternalPort: md.appConfig.HttpService.InternalPort,
			Concurrency:  concurrency,
			Ports: []api.MachinePort{
				{
					Port:       api.IntPointer(80),
					Handlers:   []string{"http"},
					ForceHttps: md.appConfig.HttpService.ForceHttps,
				},
				{
					Port:     api.IntPointer(443),
					Handlers: []string{"http", "tls"},
				},
			},
		}

		machineConfig.Services = append(machineConfig.Services, httpService)
	}

	// Copy standard services to the machine vonfig
	if md.appConfig.Services != nil {
		machineConfig.Services = append(machineConfig.Services, md.appConfig.Services...)
	}

	if md.appConfig.Env != nil {
		machineConfig.Env = md.appConfig.Env
	}

	if md.appConfig.Metrics != nil {
		machineConfig.Metrics = md.appConfig.Metrics
	}

	if md.appConfig.Checks != nil {
		machineConfig.Checks = md.appConfig.Checks
	}

	return machineConfig
}
