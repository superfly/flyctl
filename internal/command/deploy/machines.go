package deploy

import (
	"context"
	"fmt"
	"time"

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

	// Scale up if count is lower than running VM count
	if len(machines) < config.Count {
		newCount := config.Count - len(machines)
		fmt.Fprintf(io.Out, "%d VMs requested, %d running, so launching %d VMs...\n", config.Count, len(machines), newCount)
		for i := 0; i < newCount; i++ {
			_, err = flapsClient.Launch(ctx, launchInput)
		}
		fmt.Fprint(io.Out, "Completed launch.\n\n", config.Count, len(machines), newCount)
	}

	// Scale down if count is higher than running VM count
	if len(machines) > config.Count {
		killCount := len(machines) - config.Count
		fmt.Fprintf(io.Out, "%d VMs requested, %d running, so destroying %d VMs...\n", config.Count, len(machines), killCount)
		for i := 0; i < killCount; i++ {
			if machines[i].State == "started" {
				err = flapsClient.Stop(ctx, api.V1MachineStop{
					ID: machines[i].ID,
				})

				if err != nil {
					return err
				}

				// Sleep until we are able to wait for a stop
				time.Sleep(time.Second * 3)
			}

			err = flapsClient.Destroy(ctx, api.RemoveMachineInput{
				AppID: config.AppName,
				ID:    machines[i].ID,
			})

			if err != nil {
				return err
			}
		}
		fmt.Fprint(io.Out, "Finished destroying VMs.\n\n")
	}

	if err != nil {
		return
	}

	if len(machines) > 0 {
		ttl := api.IntPointer(40)

		for _, machine := range machines {
			fmt.Fprintf(io.Out, "Leasing VM %s with TTL %d\n", machine.ID, ttl)
			lease, err := flapsClient.Lease(ctx, machine.ID, ttl)

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
