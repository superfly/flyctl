package scale

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

type groupDiff struct {
	Name            string
	CurrentCount    int
	CurrentMachines []*api.Machine

	ExpectedCount    int
	MachinesToAdd    []*api.MachineConfig
	MachinesToDelete []*api.Machine
}

func runMachinesScaleCount(ctx context.Context, appName string, expectedGroupCounts map[string]int, maxPerRegion int) error {
	io := iostreams.FromContext(ctx)

	flapsClient, err := flaps.NewFromAppName(ctx, appName)
	if err != nil {
		return err
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	appConfig, err := appconfig.FromRemoteApp(ctx, appName)
	if err != nil {
		return err
	}
	regions := []string{appConfig.PrimaryRegion}

	machines, err := mach.ListActive(ctx)
	if err != nil {
		return err
	}

	machines, releaseFunc, err := mach.AcquireLeases(ctx, machines)
	defer releaseFunc(ctx, machines)
	if err != nil {
		return err
	}

	actions, err := computeActions(machines, expectedGroupCounts, regions, maxPerRegion)
	if err != nil {
		return err
	}

	for _, action := range actions {
		fmt.Fprintf(io.Out, "group:%s region:%s delta:%d\n", action.GroupName, action.Region, action.Delta)
	}

	return nil
}

type regionAction struct {
	GroupName     string
	Region        string
	MachineConfig *api.MachineConfig
	Delta         int
}

func computeActions(machines []*api.Machine, expectedGroupCounts map[string]int, regions []string, maxPerRegion int) ([]*regionAction, error) {
	// Group apps-v2 machines by process group
	machineGroups := lo.GroupBy(
		lo.Filter(machines, func(m *api.Machine, _ int) bool {
			return m.IsFlyAppsPlatform()
		}),
		func(m *api.Machine) string {
			return m.ProcessGroup()
		},
	)

	actions := make([]*regionAction, 0)

	seenGroups := make(map[string]bool)
	for groupName, groupMachines := range machineGroups {
		expected, ok := expectedGroupCounts[groupName]
		// Ignore the group if it is not expected to change or already at expected count
		if !ok {
			continue
		}
		seenGroups[groupName] = true

		actions = append(actions, &regionAction{
			GroupName:     groupName,
			Region:        regions[0],
			MachineConfig: groupMachines[0].Config,
			Delta:         expected - len(groupMachines),
		})
	}

	for name, count := range expectedGroupCounts {
		if !seenGroups[name] {
			actions = append(actions, &regionAction{
				GroupName:     name,
				Region:        regions[0],
				MachineConfig: machines[0].Config,
				Delta:         count,
			})
		}
	}

	return actions, nil
}
