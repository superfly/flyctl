package deploy

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/spinner"
	"github.com/superfly/flyctl/iostreams"
)

// Deploy ta machines app directly from flyctl, applying the desired config to running machines,
// or launching new ones
func createMachinesRelease(ctx context.Context, config *app.Config, img *imgsrc.DeploymentImage, strategy string) (err error) {
	client := client.FromContext(ctx).API()

	app, err := client.GetAppCompact(ctx, config.AppName)
	if err != nil {
		return
	}

	machineConfig := api.MachineConfig{
		Image: img.Tag,
	}

	// Convert the new, slimmer http service config to standard services
	if config.HttpService != nil {
		concurrency := config.HttpService.Concurrency

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
			InternalPort: config.HttpService.InternalPort,
			Concurrency:  concurrency,
			Ports: []api.MachinePort{
				{
					Port:       80,
					Handlers:   []string{"http"},
					ForceHttps: config.HttpService.ForceHttps,
				},
				{
					Port:     443,
					Handlers: []string{"http", "tls"},
				},
			},
		}

		machineConfig.Services = append(machineConfig.Services, httpService)
	}

	// Copy standard services to the machine vonfig
	if config.Services != nil {
		machineConfig.Services = append(machineConfig.Services, config.Services...)
	}

	if config.Env != nil {
		machineConfig.Env = config.Env
	}

	if config.Metrics != nil {
		machineConfig.Metrics = config.Metrics
	}

	if config.Checks != nil {
		machineConfig.Checks = config.Checks
	}

	// Run validations against struct types and their JSON tags
	err = config.Validate()

	if err != nil {
		return err
	}

	if err := RunReleaseCommand(ctx, app, config, machineConfig); err != nil {
		return fmt.Errorf("release command failed - aborting deployment. %w", err)
	}

	return DeployMachinesApp(ctx, app, strategy, machineConfig, config)
}

func RunReleaseCommand(ctx context.Context, app *api.AppCompact, appConfig *app.Config, machineConfig api.MachineConfig) (err error) {
	if appConfig.Deploy == nil || appConfig.Deploy.ReleaseCommand == "" {
		return nil
	}

	io := iostreams.FromContext(ctx)

	flapsClient, err := flaps.New(ctx, app)

	msg := fmt.Sprintf("Running release command: %s", appConfig.Deploy.ReleaseCommand)
	spin := spinner.Run(io, msg)
	defer spin.StopWithSuccess()

	machineConf := machineConfig

	machineConf.Metadata = map[string]string{
		"process_group": "release_command",
	}
	// Override the machine default command to run the release command
	machineConf.Init.Cmd = strings.Split(appConfig.Deploy.ReleaseCommand, " ")

	launchMachineInput := api.LaunchMachineInput{
		AppID:   app.ID,
		OrgSlug: app.Organization.ID,
		Config:  &machineConf,
	}

	// Ensure release commands run in the primary region
	if appConfig.PrimaryRegion != "" {
		launchMachineInput.Region = appConfig.PrimaryRegion
	}

	// We don't want temporary release command VMs to serve traffic, so kill the services
	machineConf.Services = nil

	machine, err := flapsClient.Launch(ctx, launchMachineInput)
	if err != nil {
		return err
	}

	removeInput := api.RemoveMachineInput{
		AppID: app.Name,
		ID:    machine.ID,
	}

	// Make sure we clean up the release command VM
	defer flapsClient.Destroy(ctx, removeInput)

	// Ensure the command starts running
	err = flapsClient.Wait(ctx, machine, "started")

	if err != nil {
		return err
	}

	// Wait for the release command VM to stop before moving on
	err = flapsClient.Wait(ctx, machine, "stopped")

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

		machine, err = flapsClient.Get(ctx, machine.ID)

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

func DeployMachinesApp(ctx context.Context, app *api.AppCompact, strategy string, machineConfig api.MachineConfig, appConfig *app.Config) (err error) {
	io := iostreams.FromContext(ctx)
	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return
	}

	if strategy == "" {
		strategy = "rolling"
	}

	var regionCode string
	if appConfig != nil {
		regionCode = appConfig.PrimaryRegion
	}

	msg := fmt.Sprintf("Deploying with %s strategy", strategy)
	spin := spinner.Run(io, msg)
	defer spin.StopWithSuccess()

	machineConfig.Metadata = map[string]string{"process_group": "app"}
	machineConfig.Init.Cmd = nil

	launchInput := api.LaunchMachineInput{
		AppID:   app.Name,
		OrgSlug: app.Organization.ID,
		Config:  &machineConfig,
		Region:  regionCode,
	}

	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return
	}

	if len(machines) > 0 {

		for _, machine := range machines {
			leaseTTL := api.IntPointer(30)
			lease, err := flapsClient.GetLease(ctx, machine.ID, leaseTTL)
			if err != nil {
				return err
			}
			machine.LeaseNonce = lease.Data.Nonce

			defer releaseLease(ctx, flapsClient, machine)
		}

		for _, machine := range machines {
			launchInput.ID = machine.ID

			// We assume a config with no image specificed means the deploy should recreate machines
			// with the existing config. For example, for applying recently set secrets.
			if machineConfig.Image == "" {
				launchInput.Config = machine.Config
			}

			launchInput.Region = machine.Region

			launchInput.Config.Checks = machine.Config.Checks

			if machine.Config.Guest != nil {
				launchInput.Config.Guest = machine.Config.Guest
			}

			// Until mounts are supported in fly.toml, ensure deployments
			// maintain any existing volume attachments
			if machine.Config.Mounts != nil {
				launchInput.Config.Mounts = machine.Config.Mounts
			}

			updateResult, err := flapsClient.Update(ctx, launchInput, machine.LeaseNonce)
			if err != nil {
				if strategy != "immediate" {
					return err

				} else {
					fmt.Printf("Continuing after error: %s\n", err)
				}
			}

			if strategy != "immediate" {
				err = flapsClient.Wait(ctx, updateResult, "started")
				if err != nil {
					return err
				}
			}
		}

	} else {
		fmt.Fprintf(io.Out, "Launching VM with image %s\n", launchInput.Config.Image)
		_, err = flapsClient.Launch(ctx, launchInput)
		if err != nil {
			return err
		}
	}

	return
}

func releaseLease(ctx context.Context, client *flaps.Client, machine *api.Machine) {
	io := iostreams.FromContext(ctx)
	if err := client.ReleaseLease(ctx, machine.ID, machine.LeaseNonce); err != nil {
		fmt.Println(io.Out, fmt.Errorf("could not release lease on %s (%w)", machine.ID, err))
	}
}
