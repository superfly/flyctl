package deploy

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/flaps"
	"github.com/superfly/flyctl/pkg/iostreams"
)

// Deploy ta machines app directly from flyctl, applying the desired config to running machines,
// or launching new ones
func createMachinesRelease(ctx context.Context, config *app.Config, img *imgsrc.DeploymentImage) (err error) {
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

	if config.HttpService != nil {
		machineConfig.Services = []api.MachineService{
			{
				Protocol:     "tcp",
				InternalPort: config.HttpService.InternalPort,
				Ports: []api.MachinePort{
					{
						Port:       80,
						Handlers:   []string{"http"},
						ForceHttps: true,
					},
				},
			},
			{
				Protocol:     "tcp",
				InternalPort: config.HttpService.InternalPort,
				Ports: []api.MachinePort{
					{
						Port:     443,
						Handlers: []string{"http", "tls"},
					},
				},
			},
		}
	}

	err = config.Validate()

	if err != nil {
		return err
	}

	launchInput := api.LaunchMachineInput{
		AppID:   config.AppName,
		OrgSlug: app.Organization.ID,
		Region:  config.PrimaryRegion,
		Config:  machineConfig,
	}

	machines, err := flapsClient.List(ctx, "")

	if err != nil {
		return
	}

	if len(machines) > 0 {
		ttl := api.IntPointer(40)

		for _, machine := range machines {
			fmt.Fprintf(io.Out, "Leasing VM %s with TTL %d\n", machine.ID, ttl)
			lease, err := flapsClient.GetLease(ctx, machine.ID, ttl)

			if err != nil {
				return err
			}

			machine.LeaseNonce = lease.Data.Nonce
		}

		for _, machine := range machines {

			fmt.Fprintf(io.Out, "Updating VM %s\n", machine.ID)
			launchInput.ID = machine.ID
			flapsClient.Update(ctx, launchInput)

		}

		for _, machine := range machines {
			fmt.Fprintf(io.Out, "Releasing VM %s with nonce %s\n", machine.ID, machine.LeaseNonce)

			err = flapsClient.ReleaseLease(ctx, machine.ID, machine.LeaseNonce)

			if err != nil {
				fmt.Fprintf(io.Out, "Could not release lease %s on machine %s. Error: %s, Continuing.", machine.LeaseNonce, machine.ID, err)
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
