package builder

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/iostreams"
)

func GetMachine(ctx context.Context, orgSlug string) (builder *api.Machine, err error) {
	client := client.FromContext(ctx).API()
	out := iostreams.FromContext(ctx).Out

	org, err := client.GetOrganizationBySlug(ctx, orgSlug)

	builderApp := org.RemoteBuilderApp

	if err != nil {
		return
	}

	if org.RemoteBuilderApp == nil {
		fmt.Fprintf(out, "organization %s has no remote builder app, so starting one now", orgSlug)
		_, builderApp, err = LaunchOrWake(ctx, orgSlug)

		if err != nil {
			return nil, err
		}
	}
	flapsClient, err := flaps.New(ctx, builderApp)

	if err != nil {
		return
	}

	machines, err := flapsClient.List(ctx, "")

	if len(machines) < 1 {
		return nil, fmt.Errorf("builder app %s has no machines", org.RemoteBuilderApp.Name)
	} else {
		builder = machines[0]
	}
	return
}

func LaunchOrWake(ctx context.Context, orgSlug string) (builder *api.Machine, builderApp *api.AppCompact, err error) {
	out := iostreams.FromContext(ctx).Out
	client := client.FromContext(ctx).API()

	org, err := client.GetOrganizationBySlug(ctx, orgSlug)

	if err != nil {
		return nil, nil, err
	}

	builderApp = org.RemoteBuilderApp

	if builderApp == nil {
		builderApp, err = client.CreateApp(ctx, api.CreateAppInput{
			OrganizationID: org.ID,
			AppRoleID:      "remote-docker-builder",
			Machines:       true,
		})

	}

	if err != nil {
		return
	}

	var builderVolume *api.Volume

	volumes, err := client.GetVolumes(ctx, builderApp.Name)

	if len(volumes) > 0 {
		builderVolume = &volumes[0]
	} else {
		region, err := client.GetNearestRegion(ctx)

		if err != nil {
			return nil, nil, err
		}

		builderVolume, err = client.CreateVolume(ctx, api.CreateVolumeInput{
			AppID:  builderApp.ID,
			Name:   "builder_data",
			SizeGb: 50,
			Region: region.Code,
		})

		if err != nil {
			return nil, nil, err
		}

	}

	if err != nil {
		return
	}

	flapsClient, err := flaps.New(ctx, builderApp)

	if err != nil {
		return
	}

	machines, err := flapsClient.List(ctx, "")

	// We found a machine, so start or wake it
	if len(machines) > 0 {
		builder = machines[0]
		if builder.State == "started" {
			flapsClient.Wake(ctx, builder.ID)
		} else {
			flapsClient.Start(ctx, builder.ID)
		}

	} else {

		region, err := client.GetNearestRegion(ctx)

		if err != nil {
			return nil, nil, err
		}

		builderVolumeConf := api.MachineMount{
			Path:   "/data",
			Volume: builderVolume.Name,
		}

		input := api.LaunchMachineInput{
			AppID:  builderApp.ID,
			Region: region.Code,
			Config: &api.MachineConfig{
				Image:  "flyio/rchab:sha-58e72ae",
				Mounts: []api.MachineMount{builderVolumeConf},
			},
		}

		builder, err = flapsClient.Launch(ctx, input)

		if err != nil {
			return nil, nil, err
		}
	}

	fmt.Fprintf(out, "Builder instance %s is ready\n", builder.ID)

	return
}
