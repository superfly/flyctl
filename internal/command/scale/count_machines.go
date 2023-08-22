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
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

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

	volumes, err := flapsClient.GetVolumes(ctx)
	if err != nil {
		return err
	}
	defaults := newDefaults(appConfig, latestCompleteRelease, machines, volumes)

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
		fmt.Fprintf(io.Out, "%+4d machines for group '%s' on region '%s' of size '%s'\n",
			action.Delta, action.GroupName, action.Region, action.MachineSize())

		volumesToReuse := len(action.Volumes)
		volumesToCreate := action.VolumesDelta()
		switch {
		case volumesToReuse > 0 && volumesToCreate > 0:
			fmt.Fprintf(io.Out, "%+4d volumes and using %d existing volumes for group '%s' in region '%s'\n", volumesToCreate, volumesToReuse, action.GroupName, action.Region)
		case volumesToReuse > 0:
			fmt.Fprintf(io.Out, "  Using %d existing volumes for group '%s' in region '%s'\n", volumesToReuse, action.GroupName, action.Region)
		case volumesToCreate > 0:
			fmt.Fprintf(io.Out, "%+4d volumes for group '%s' in region '%s'\n", volumesToCreate, action.GroupName, action.Region)
		}
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
	for _, action := range actions {
		switch {
		case action.Delta > 0:
			for i := 0; i < action.Delta; i++ {
				m, err := launchMachine(ctx, action, i)
				if err != nil {
					return err
				}

				fmt.Fprintf(io.Out, "  Created %s group:%s region:%s size:%s",
					m.ID, action.GroupName, action.Region, m.Config.Guest.ToSize(),
				)
				if len(m.Config.Mounts) > 0 {
					fmt.Fprintf(io.Out, " volume:%s", m.Config.Mounts[0].Volume)
				}
				fmt.Fprintln(io.Out)
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

func launchMachine(ctx context.Context, action *planItem, idx int) (*api.Machine, error) {
	flapsClient := flaps.FromContext(ctx)
	input := helpers.Clone(*action.LaunchMachineInput)

	if len(input.Config.Mounts) > 0 {
		var volume *api.Volume
		if idx < len(action.Volumes) {
			volume = action.Volumes[idx]
		} else if action.CreateVolumeRequest != nil {
			var err error
			volume, err = flapsClient.CreateVolume(ctx, *action.CreateVolumeRequest)
			if err != nil {
				return nil, err
			}
		}
		input.Config.Mounts[0].Volume = volume.ID
	}

	return flapsClient.Launch(ctx, input)
}

func pickOrCreateVolume(ctx context.Context, mount api.MachineMount, volumes []*api.Volume, region string) (*api.Volume, error) {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	flapsClient := flaps.FromContext(ctx)

	if !flag.GetBool(ctx, "with-new-volumes") {
		for _, av := range volumes {
			fmt.Fprintf(io.Out, "  Using unattached volume %s\n", colorize.Bold(av.ID))
			return av, nil
		}
	}

	var snapshotID *string
	switch snapID := flag.GetString(ctx, "from-snapshot"); snapID {
	case "last":
		snapshots, err := flapsClient.GetVolumeSnapshots(ctx, mount.Volume)
		if err != nil {
			return nil, err
		}
		if len(snapshots) > 0 {
			snapshot := lo.MaxBy(snapshots, func(i, j api.VolumeSnapshot) bool { return i.CreatedAt.After(j.CreatedAt) })
			snapshotID = &snapshot.ID
			fmt.Fprintf(io.Out, "  Creating new volume from snapshot %s of %s\n", colorize.Bold(*snapshotID), colorize.Bold(mount.Volume))
		} else {
			fmt.Fprintf(io.Out, "  No snapshot for source volume %s, the new volume will start empty\n", colorize.Bold(mount.Volume))
			snapshotID = nil
		}
	case "":
		fmt.Fprintf(io.Out, "  Volume %s will start empty\n", colorize.Bold(mount.Name))
	default:
		snapshotID = &snapID
		fmt.Fprintf(io.Out, "  Creating new volume from snapshot: %s\n", colorize.Bold(*snapshotID))
	}

	volInput := api.CreateVolumeRequest{
		Name:              mount.Name,
		Region:            region,
		SizeGb:            &mount.SizeGb,
		Encrypted:         api.Pointer(mount.Encrypted),
		SnapshotID:        snapshotID,
		RequireUniqueZone: api.Pointer(false),
	}

	return flapsClient.CreateVolume(ctx, volInput)
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
	GroupName string
	Region    string
	// The number of machines to add or remove
	Delta              int
	Machines           []*api.Machine
	MachineConfig      *api.MachineConfig
	LaunchMachineInput *api.LaunchMachineInput
	// Volumes to reuse
	Volumes []*api.Volume
	// Input used to create new volumes
	CreateVolumeRequest *api.CreateVolumeRequest
}

func (pi *planItem) VolumesDelta() int {
	return pi.Delta - len(pi.Volumes)
}

func (pi *planItem) MachineSize() string {
	if pi.Delta > 0 {
		return pi.LaunchMachineInput.Config.Guest.ToSize()
	}
	if len(pi.Machines) > 0 {
		return pi.Machines[0].Config.Guest.ToSize()
	}
	return ""
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

		for region, delta := range regionDiffs {
			actions = append(actions, &planItem{
				GroupName:           groupName,
				Region:              region,
				Delta:               delta,
				Machines:            perRegionMachines[region],
				LaunchMachineInput:  &api.LaunchMachineInput{Region: region, Config: mConfig},
				Volumes:             defaults.PopAvailableVolumes(mConfig, region, delta),
				CreateVolumeRequest: defaults.CreateVolumeRequest(mConfig, region, delta),
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

		for region, delta := range regionDiffs {
			actions = append(actions, &planItem{
				GroupName:           groupName,
				Region:              region,
				Delta:               delta,
				LaunchMachineInput:  &api.LaunchMachineInput{Region: region, Config: mConfig},
				Volumes:             defaults.PopAvailableVolumes(mConfig, region, delta),
				CreateVolumeRequest: defaults.CreateVolumeRequest(mConfig, region, delta),
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
