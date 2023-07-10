package scale

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

type trackedVolume struct {
	volume api.Volume
	used   bool
}

func runMachinesScaleCount(ctx context.Context, appName string, appConfig *appconfig.Config, expectedGroupCounts map[string]int, maxPerRegion int) error {
	io := iostreams.FromContext(ctx)
	flapsClient := flaps.FromContext(ctx)
	ctx = appconfig.WithConfig(ctx, appConfig)
	apiClient := client.FromContext(ctx).API()

	machines, _, err := flapsClient.ListFlyAppsMachines(ctx)
	if err != nil {
		return err
	}

	var latestCompleteRelease api.Release

	releases, err := apiClient.GetAppReleasesMachines(ctx, appName, "complete", 1)
	if err != nil {
		return err
	}

	if len(releases) > 0 {
		latestCompleteRelease = releases[0]
	} else {
		return fmt.Errorf("this app has no complete releases. Run `fly deploy` to create one and rerun this command")
	}

	var regions []string
	if v := flag.GetRegion(ctx); v != "" {
		regions = strings.Split(v, ",")
	}
	if len(regions) == 0 {
		regions = lo.Uniq(lo.Map(machines, func(m *api.Machine, _ int) string { return m.Region }))
		if len(regions) == 0 {
			regions = []string{appConfig.PrimaryRegion}
		}
	}

	machines, releaseFunc, err := mach.AcquireLeases(ctx, machines)
	defer releaseFunc(ctx, machines)
	if err != nil {
		return err
	}

	defaults := newDefaults(appConfig, latestCompleteRelease, machines)

	actions, err := computeActions(machines, expectedGroupCounts, regions, maxPerRegion, defaults)
	if err != nil {
		return err
	}

	if len(actions) == 0 {
		fmt.Fprintf(io.Out, "App already scaled to desired state. No need for changes\n")
		return nil
	}

	fmt.Fprintf(io.Out, "App '%s' is going to be scaled according to this plan:\n", appName)

	for _, action := range actions {
		size := action.MachineConfig.Guest.ToSize()
		fmt.Fprintf(io.Out, "%+4d machines for group '%s' on region '%s' with size '%s'\n", action.Delta, action.GroupName, action.Region, size)
	}

	if !flag.GetYes(ctx) {
		switch confirmed, err := prompt.Confirmf(ctx, "Scale app %s?", appName); {
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
	volumes, err := apiClient.GetVolumes(ctx, appName)
	if err != nil {
		return err
	}

	trackedVolumes := lo.FilterMap(volumes, func(v api.Volume, _ int) (*trackedVolume, bool) {
		if !v.IsAttached() {
			tv := trackedVolume{
				volume: v,
				used:   false,
			}
			return &tv, true
		}

		return &trackedVolume{}, false
	})

	for _, action := range actions {
		switch {
		case action.Delta > 0:
			for i := 0; i < action.Delta; i++ {
				m, err := launchMachine(ctx, action, appConfig, trackedVolumes)
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

func launchMachine(ctx context.Context, action *planItem, appConfig *appconfig.Config, volumes []*trackedVolume) (*api.Machine, error) {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	flapsClient := flaps.FromContext(ctx)
	apiClient := client.FromContext(ctx).API()

	app, err := apiClient.GetAppCompact(ctx, appConfig.AppName)
	if err != nil {
		return nil, err
	}

	if len(action.MachineConfig.Mounts) > 0 {
		// Is this ever more than 1?
		for _, mnt := range action.MachineConfig.Mounts {
			var vol *api.Volume
			availableVolumes := lo.Filter(volumes, func(tv *trackedVolume, _ int) bool {
				return tv.volume.Region == action.Region && !tv.used
			})
			if len(availableVolumes) > 0 {
				trackedVolume := availableVolumes[0]
				trackedVolume.used = true

				vol = &trackedVolume.volume
				fmt.Fprintf(io.Out, "Using unattached volume '%s'", colorize.Bold(vol.ID))
			} else {
				fmt.Fprintf(io.Out, "Creating new volume '%s'. Will start empty\n", colorize.Bold(mnt.Name))

				volInput := api.CreateVolumeInput{
					AppID:             app.ID,
					Name:              mnt.Name,
					Region:            action.Region,
					SizeGb:            mnt.SizeGb,
					Encrypted:         mnt.Encrypted,
					SnapshotID:        nil,
					RequireUniqueZone: false,
				}
				vol, err = apiClient.CreateVolume(ctx, volInput)
				if err != nil {
					return nil, err
				}
			}

			action.MachineConfig.Mounts = []api.MachineMount{
				{
					Volume: vol.ID,
					Path:   mnt.Path,
				},
			}
		}
	}

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
