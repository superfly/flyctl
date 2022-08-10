package deploy

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/build/imgsrc"
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

	machineConfig := &api.MachineConfig{
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

	// Run validations against struct types and their JSON tags
	err = config.Validate()

	if err != nil {
		return err
	}

	err = RunReleaseCommand(ctx, app, config, machineConfig)

	if err != nil {
		return fmt.Errorf("release command failed - aborting deployment. %w", err)
	}

	return DeployMachinesApp(ctx, app, strategy, machineConfig)
}

func RunReleaseCommand(ctx context.Context, app *api.AppCompact, appConfig *app.Config, machineConf *api.MachineConfig) (err error) {
	out := iostreams.FromContext(ctx).Out

	flapsClient, err := flaps.New(ctx, app)

	if appConfig.Deploy.ReleaseCommand == "" {
		return
	}

	fmt.Fprintf(out, "Running release command: %s", appConfig.Deploy.ReleaseCommand)
	// Override the machine default command to run the release command
	machineConf.Init.Cmd = strings.Split(appConfig.Deploy.ReleaseCommand, " ")

	// We don't want temporary release command VMs to serve traffic
	machineConf.Services = nil

	machine, err := flapsClient.Launch(ctx,
		api.LaunchMachineInput{
			AppID:   app.ID,
			OrgSlug: app.Organization.ID,
			Config:  machineConf,
		},
	)

	if err != nil {
		return err
	}

	removeInput := api.RemoveMachineInput{
		AppID: app.Name,
		ID:    machine.ID,
		Kill:  true,
	}

	defer flapsClient.Destroy(ctx, removeInput)

	err = flapsClient.Wait(ctx, machine, "started")

	if err != nil {
		return err
	}

	err = flapsClient.Wait(ctx, machine, "stopped")

	if err != nil {
		return fmt.Errorf("Failed determining whether the release command finished. %w", err)
	}

	machine, err = flapsClient.Get(ctx, machine.ID)

	var lastExitEvent *api.MachineEvent

	for _, event := range machine.Events {
		if event.Type != "exit" {
			continue
		}

		if lastExitEvent == nil || event.Timestamp > lastExitEvent.Timestamp {
			lastExitEvent = event
		}
	}

	exitCode := lastExitEvent.Request.ExitEvent.ExitCode

	if exitCode != 0 {
		return fmt.Errorf("Release command exited with non-zero status of %d", exitCode)
	}

	return
}

func DeployMachinesApp(ctx context.Context, app *api.AppCompact, strategy string, machineConfig *api.MachineConfig) (err error) {
	out := iostreams.FromContext(ctx).Out
	flapsClient, err := flaps.New(ctx, app)

	if strategy == "" {
		strategy = "rolling"
	}

	fmt.Fprintf(out, "Deploying with %s strategy\n", strategy)

	if err != nil {
		return
	}

	launchInput := api.LaunchMachineInput{
		AppID:   app.Name,
		OrgSlug: app.Organization.ID,
		Config:  machineConfig,
	}

	machines, err := flapsClient.List(ctx, "")

	if err != nil {
		return
	}

	if len(machines) > 0 {

		for _, machine := range machines {

			fmt.Fprintf(out, "Taking lease out on VM %s\n", machine.ID)
			leaseTTL := api.IntPointer(30)
			lease, err := flapsClient.GetLease(ctx, machine.ID, leaseTTL)

			if err != nil {
				return err
			}

			machine.LeaseNonce = lease.Data.Nonce

		}

		for _, machine := range machines {

			fmt.Fprintf(out, "Updating VM %s\n", machine.ID)

			launchInput.ID = machine.ID

			// We assume an empty config means the deploy should simply recreate machines with the existing config,
			// for example for applying recently set secrets

			if launchInput.Config == nil {
				freshMachine, err := flapsClient.Get(ctx, machine.ID)

				if err != nil {
					return err
				}

				launchInput.Config = freshMachine.Config
				launchInput.Region = machine.Region
			}

			// Until mounts are supported in fly.toml, ensure deployments
			// maintain any existing volume attachments
			if machine.Config.Mounts != nil {
				launchInput.Config.Mounts = append(launchInput.Config.Mounts, machine.Config.Mounts[0])
			}

			updateResult, err := flapsClient.Update(ctx, launchInput, machine.LeaseNonce)

			if err != nil {

				if strategy != "immediate" {
					leaseErr := flapsClient.ReleaseLease(ctx, machine.ID, machine.LeaseNonce)

					if leaseErr != nil {
						fmt.Fprintf(out, "Could not release lease for %s\n", machine.ID)
					}

					return err

				} else {
					fmt.Printf("Continuing after error: %s\n", err)
				}
			}

			if strategy != "immediate" {
				fmt.Fprintf(out, "Waiting for update to finish on %s\n", machine.ID)
				err = flapsClient.Wait(ctx, updateResult, "started")

				if err != nil {
					err = flapsClient.ReleaseLease(ctx, machine.ID, machine.LeaseNonce)
					return err
				}
			}

		}

		for _, machine := range machines {

			fmt.Fprintf(out, "Releasing lease on %s\n", machine.ID)
			err = flapsClient.ReleaseLease(ctx, machine.ID, machine.LeaseNonce)
			if err != nil {
				fmt.Println(out, fmt.Errorf("could not release lease on %s (%w)", machine.ID, err))
			}

		}

		fmt.Fprintln(out)

	} else {
		fmt.Fprintf(out, "Launching VM with image %s\n", launchInput.Config.Image)
		_, err = flapsClient.Launch(ctx, launchInput)

		if err != nil {
			return err
		}
	}

	fmt.Fprintln(out, "Deploy complete. Check the result with 'fly status'.")

	return
}
