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
						Port:     80,
						Handlers: []string{"http"},
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

	machines, err := flapsClient.List(ctx, "started")

	if err != nil {
		return
	}

	if len(machines) > 0 {
		for _, machine := range machines {
			fmt.Fprintf(io.Out, "Updating machine %s\n", machine.ID)
			launchInput.ID = machine.ID
			flapsClient.Update(ctx, launchInput)
		}

	} else {
		_, err = flapsClient.Launch(ctx, launchInput)
	}

	return
}
