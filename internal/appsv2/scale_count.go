package appsv2

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

type ScaleCountInput struct {
	AppName             string
	AppConfig           *appconfig.Config
	ExpectedGroupCounts map[string]int
	MaxPerRegion        int
	Regions             []string
	AutoConfirm         bool
}

func ScaleCount(ctx context.Context, input ScaleCountInput) error {
	io := iostreams.FromContext(ctx)
	ctx = appconfig.WithConfig(ctx, input.AppConfig)

	flapsClient, err := flaps.NewFromAppName(ctx, input.AppName)
	if err != nil {
		return err
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	machines, _, err := flapsClient.ListFlyAppsMachines(ctx)
	if err != nil {
		return err
	}

	regions := input.Regions
	if len(regions) == 0 {
		regions = lo.Uniq(lo.Map(machines, func(m *api.Machine, _ int) string {
			return m.Region
		}))
	}

	if len(machines) == 0 {
		// We need at least one machine to grab the image to use.
		// FIXME: fetch image, release id and version from latest "complete" release
		return fmt.Errorf("there are no active machines for this app. Run `fly deploy` to create one and rerun this command")
	}

	machines, releaseFunc, err := mach.AcquireLeases(ctx, machines)
	defer releaseFunc(ctx, machines)
	if err != nil {
		return err
	}

	defaults := newDefaults(input.AppConfig, machines)

	actions, err := computeActions(machines, input.ExpectedGroupCounts, regions, input.MaxPerRegion, defaults)
	if err != nil {
		return err
	}

	if len(actions) == 0 {
		fmt.Fprintf(io.Out, "App already scaled to desired state. No need for changes\n")
		return nil
	}

	fmt.Fprintf(io.Out, "App '%s' is going to be scaled according to this plan:\n", input.AppName)

	needsVolumes := map[string]bool{}
	for _, action := range actions {
		size := action.MachineConfig.Guest.ToSize()
		fmt.Fprintf(io.Out, "%+4d machines for group '%s' on region '%s' with size '%s'\n", action.Delta, action.GroupName, action.Region, size)
		if len(action.MachineConfig.Mounts) > 0 && action.Delta > 0 {
			needsVolumes[action.GroupName] = true
		}
	}

	if len(needsVolumes) > 0 {
		groupNames := maps.Keys(needsVolumes)
		slices.Sort(groupNames)
		return fmt.Errorf(
			"'fly scale count' can't scale up groups with mounts, "+
				"use 'fly machine clone' to add machines for: %s",
			strings.Join(groupNames, " "),
		)
	}

	if !flag.GetYes(ctx) {
		switch confirmed, err := prompt.Confirmf(ctx, "Scale app %s?", input.AppName); {
		case err == nil:
			if !confirmed {
				return nil
			}
		case prompt.IsNonInteractive(err):
			return prompt.NonInteractiveError("--yes flag must be specified when not running interactively")
		default:
			return err
		}
	}

	fmt.Fprintf(io.Out, "Executing scale plan\n")
	for _, action := range actions {
		switch {
		case action.Delta > 0:
			for i := 0; i < action.Delta; i++ {
				m, err := launchMachine(ctx, action)
				if err != nil {
					return err
				}
				fmt.Fprintf(io.Out, "  Created %s group:%s region:%s size:%s\n", m.ID, action.GroupName, action.Region, m.Config.Guest.ToSize())
			}
		case action.Delta < 0:
			for i := 0; i > action.Delta; i-- {
				m := action.Machines[-i]
				err := destroyMachine(ctx, m)
				if err != nil {
					return err
				}
				fmt.Fprintf(io.Out, "  Destroyed %s group:%s region:%s size:%s\n", m.ID, action.GroupName, action.Region, m.Config.Guest.ToSize())
			}
		}
	}

	return nil
}

func launchMachine(ctx context.Context, action *planItem) (*api.Machine, error) {
	flapsClient := flaps.FromContext(ctx)

	input := api.LaunchMachineInput{
		Region: action.Region,
		Config: action.MachineConfig,
	}
	return flapsClient.Launch(ctx, input)
}

func destroyMachine(ctx context.Context, machine *api.Machine) error {
	flapsClient := flaps.FromContext(ctx)
	input := api.RemoveMachineInput{
		ID:   machine.ID,
		Kill: true,
	}
	return flapsClient.Destroy(ctx, input, machine.LeaseNonce)
}

type planItem struct {
	GroupName     string
	Region        string
	Delta         int
	Machines      []*api.Machine
	MachineConfig *api.MachineConfig
}

func computeActions(machines []*api.Machine, expectedGroupCounts map[string]int, regions []string, maxPerRegion int, defaults *defaultValues) ([]*planItem, error) {
	actions := make([]*planItem, 0)
	seenGroups := make(map[string]bool)
	machineGroups := lo.GroupBy(machines, func(m *api.Machine) string {
		return m.ProcessGroup()
	})

	for groupName, groupMachines := range machineGroups {
		expected, ok := expectedGroupCounts[groupName]
		// Ignore the group if it is not expected to change
		if !ok {
			continue
		}
		seenGroups[groupName] = true

		perRegionMachines := lo.GroupBy(groupMachines, func(m *api.Machine) string {
			return m.Region
		})

		currentPerRegionCount := lo.MapEntries(perRegionMachines, func(k string, v []*api.Machine) (string, int) {
			return k, len(v)
		})

		regionDiffs, err := convergeGroupCounts(expected, currentPerRegionCount, regions, maxPerRegion)
		if err != nil {
			return nil, err
		}

		mConfig := groupMachines[0].Config
		// Nullify standbys, no point on having more than one
		mConfig.Standbys = nil

		for regionName, delta := range regionDiffs {
			actions = append(actions, &planItem{
				GroupName:     groupName,
				Region:        regionName,
				Delta:         delta,
				Machines:      perRegionMachines[regionName],
				MachineConfig: mConfig,
			})
		}
	}

	// Fill in the groups without existing machines
	for groupName, expected := range expectedGroupCounts {
		if seenGroups[groupName] {
			continue
		}

		mConfig, err := defaults.ToMachineConfig(groupName)
		if err != nil {
			return nil, err
		}

		regionDiffs, err := convergeGroupCounts(expected, nil, regions, maxPerRegion)
		if err != nil {
			return nil, err
		}

		for regionName, delta := range regionDiffs {
			actions = append(actions, &planItem{
				GroupName:     groupName,
				Region:        regionName,
				Delta:         delta,
				MachineConfig: mConfig,
			})
		}
	}

	return actions, nil
}

var MaxPerRegionError = errors.New("the number of regions by the maximum machines per region is fewer than the expected total")

func convergeGroupCounts(expectedTotal int, current map[string]int, regions []string, maxPerRegion int) (map[string]int, error) {
	diffs := make(map[string]int)

	if len(regions) == 0 {
		regions = lo.Keys(current)
	}

	if maxPerRegion >= 0 {
		if len(regions)*maxPerRegion < expectedTotal {
			return nil, MaxPerRegionError
		}

		// Compute the diff to any region with more machines than the maximum allowed
		for _, region := range regions {
			c := current[region]
			if c > maxPerRegion {
				diffs[region] = maxPerRegion - c
			}
		}
	}

	diff := expectedTotal
	for _, region := range regions {
		diff -= (current[region] + diffs[region])
	}

	idx := 0
	for diff > 0 {
		region := regions[idx%(len(regions))]
		if maxPerRegion < 0 || current[region]+diffs[region] < maxPerRegion {
			diffs[region]++
			diff--
		}
		idx++
	}

	// Iterate regions in reverse order because the region list
	// tend to have the primary region first
	idx = -1
	for diff < 0 {
		region := regions[-idx%(len(regions))]
		if current[region]+diffs[region] > 0 {
			diffs[region]--
			diff++
		}
		idx--
	}

	return diffs, nil
}
