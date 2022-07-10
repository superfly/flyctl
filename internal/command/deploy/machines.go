package deploy

import (
	"context"
	"fmt"

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
	io := iostreams.FromContext(ctx)

	client := client.FromContext(ctx).API()

	app, err := client.GetAppCompact(ctx, config.AppName)

	if err != nil {
		return
	}

	flapsClient, err := flaps.New(ctx, app)

	if err != nil {
		return
	}

	machineConfig := &api.MachineConfig{
		Image: img.Tag,
	}

	// Convert the new, slimmer http service config to standard services
	if config.HttpService != nil {

		httpService := api.MachineService{
			Protocol:     "tcp",
			InternalPort: config.HttpService.InternalPort,
			Ports: []api.MachinePort{
				{
					Port:       80,
					Handlers:   []string{"http"},
					ForceHttps: true,
				},
			},
		}

		httpsService := api.MachineService{
			Protocol:     "tcp",
			InternalPort: config.HttpService.InternalPort,
			Ports: []api.MachinePort{
				{
					Port:     443,
					Handlers: []string{"http", "tls"},
				},
			},
		}

		machineConfig.Services = append(machineConfig.Services, httpService, httpsService)
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

	launchInput := api.LaunchMachineInput{
		AppID:   config.AppName,
		OrgSlug: app.Organization.ID,
		Region:  config.GetPrimaryRegion(),
		Config:  machineConfig,
	}

	machines, err := flapsClient.List(ctx, "")

	if err != nil {
		return
	}

	if len(machines) > 0 {

		for _, machine := range machines {

			fmt.Fprintf(io.Out, "Taking lease out on VM %s\n", machine.ID)
			launchInput.ID = machine.ID
			leaseTTL := api.IntPointer(30)
			lease, err := flapsClient.GetLease(ctx, machine.ID, leaseTTL)

			if err != nil {
				return err
			}

			machine.LeaseNonce = lease.Data.Nonce

		}

		for _, machine := range machines {

			fmt.Fprintf(io.Out, "Updating VM %s\n", machine.ID)

			launchInput.ID = machine.ID

			// Until mounts are supported in fly.toml, ensure deployments
			// maintain any existing volume attachments
			if machine.Config.Mounts != nil {
				launchInput.Config.Mounts = append(launchInput.Config.Mounts, machine.Config.Mounts[0])
			}

			updateResult, err := flapsClient.Update(ctx, launchInput, machine.LeaseNonce)

			if err != nil && strategy == "immediate" {
				fmt.Printf("Continuing after error: %s\n", err)
			} else if err != nil {
				return err
			}

			if strategy != "immediate" {
				fmt.Fprintf(io.Out, "Waiting for update to finish on %s\n", machine.ID)
				err = flapsClient.Wait(ctx, updateResult)

				if err != nil {
					return err
				}
			}

		}

		for _, machine := range machines {

			fmt.Fprintf(io.Out, "Releasing lease on %s\n", machine.ID)
			err = flapsClient.ReleaseLease(ctx, machine.ID, machine.LeaseNonce)
			if err != nil {
				return err
			}

		}
		fmt.Fprintln(io.Out)

	} else {
		fmt.Fprintf(io.Out, "Launching VM with image %s\n", launchInput.Config.Image)
		_, err = flapsClient.Launch(ctx, launchInput)

		if err != nil {
			return err
		}
	}

	fmt.Fprintln(io.Out, "Deploy complete. Check the result with 'fly status'.")

	return
}
