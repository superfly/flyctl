package scale

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	mach "github.com/superfly/flyctl/internal/machine"
)

func v2ScaleVM(ctx context.Context, appName, group, sizeName string, memoryMB int) (*api.VMSize, error) {
	flapsClient, err := flaps.NewFromAppName(ctx, appName)
	if err != nil {
		return nil, err
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	// Quickly validate sizeName before any network call
	if err := (&api.MachineGuest{}).SetSize(sizeName); err != nil && sizeName != "" {
		return nil, err
	}

	if group == "" {
		appConfig, err := appconfig.FromRemoteApp(ctx, appName)
		if err != nil {
			return nil, err
		}
		if len(appConfig.Processes) > 1 {
			return nil, fmt.Errorf("scaling an app with multiple process groups requires specifying a group with '--group <name>'\n * this app has the following process groups: %v", appConfig.FormatProcessNames())
		}
		group = appConfig.DefaultProcessName()
	}

	machines, err := listMachinesWithGroup(ctx, group)
	if err != nil {
		return nil, err
	}
	if len(machines) == 0 {
		return nil, fmt.Errorf("No active machines in process group '%s', check `fly status` output", group)
	}

	machines, releaseFunc, err := mach.AcquireLeases(ctx, machines)
	defer releaseFunc(ctx, machines)
	if err != nil {
		return nil, err
	}

	for _, machine := range machines {
		if sizeName != "" {
			machine.Config.Guest.SetSize(sizeName)
		}
		if memoryMB > 0 {
			machine.Config.Guest.MemoryMB = memoryMB
		}

		input := &api.LaunchMachineInput{
			ID:               machine.ID,
			AppID:            appName,
			Name:             machine.Name,
			Region:           machine.Region,
			Config:           machine.Config,
			SkipHealthChecks: false,
		}
		if err := mach.Update(ctx, machine, input); err != nil {
			return nil, err
		}
	}

	// Return api.VMSize to remain compatible with v1 scale app signature
	size := &api.VMSize{
		Name:     machines[0].Config.Guest.ToSize(),
		MemoryMB: machines[0].Config.Guest.MemoryMB,
		CPUCores: float32(machines[0].Config.Guest.CPUs),
	}

	return size, nil
}

func listMachinesWithGroup(ctx context.Context, group string) ([]*api.Machine, error) {
	machines, err := mach.ListActive(ctx)
	if err != nil {
		return nil, err
	}

	machines = lo.Filter(machines, func(m *api.Machine, _ int) bool {
		return m.ProcessGroup() == group
	})

	return machines, nil
}
