package scale

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

const maxConcurrentActions = 5

func runMachinesScaleCount(ctx context.Context, appName string, appConfig *appconfig.Config, expectedGroupCounts groupCounts, maxPerRegion int) error {
	io := iostreams.FromContext(ctx)
	flapsClient := flapsutil.ClientFromContext(ctx)
	ctx = appconfig.WithConfig(ctx, appConfig)
	apiClient := flyutil.ClientFromContext(ctx)

	machines, _, err := flapsClient.ListFlyAppsMachines(ctx)
	if err != nil {
		return err
	}

	machines = lo.Filter(machines, func(m *fly.Machine, _ int) bool {
		return m.Config != nil
	})

	var latestCompleteRelease fly.Release
	switch releases, err := apiClient.GetAppReleasesMachines(ctx, appName, "complete", 1); {
	case err != nil:
		return err
	case len(releases) == 0:
		return fmt.Errorf("this app has no complete releases. Run `fly deploy` to create one and rerun this command")
	default:
		latestCompleteRelease = releases[0]
	}

	var regions []string
	if v := flag.GetRegion(ctx); v != "" {
		regions = strings.Split(v, ",")
	}
	if len(regions) == 0 {
		regions = lo.Uniq(lo.Map(machines, func(m *fly.Machine, _ int) string { return m.Region }))
		if len(regions) == 0 {
			regions = []string{appConfig.PrimaryRegion}
		}
	}

	volumes, err := flapsClient.GetVolumes(ctx)
	if err != nil {
		return err
	}

	defaultGuest, err := flag.GetMachineGuest(ctx, nil)
	if err != nil {
		return err
	}

	defaults := newDefaults(appConfig, latestCompleteRelease, machines, volumes,
		flag.GetString(ctx, "from-snapshot"), flag.GetBool(ctx, "with-new-volumes"), defaultGuest)

	actions, err := computeActions(machines, expectedGroupCounts, regions, maxPerRegion, defaults)
	if err != nil {
		return err
	}

	// Add env variable overrides to launch configs
	if env := flag.GetStringArray(ctx, "env"); len(env) > 0 {
		parsedEnv, err := cmdutil.ParseKVStringsToMap(env)
		if err != nil {
			return fmt.Errorf("failed parsing environment: %w", err)
		}
		lo.ForEach(actions, func(plan *planItem, _ int) {
			c := plan.LaunchMachineInput.Config
			c.Env = lo.Assign(c.Env, parsedEnv)
		})
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
			fmt.Fprintf(io.Out, "%+4d volumes and %d unattached volumes assigned to group '%s' in region '%s'\n", volumesToCreate, volumesToReuse, action.GroupName, action.Region)
		case volumesToReuse > 0:
			fmt.Fprintf(io.Out, "% 4d unattached volumes to be assigned to group '%s' in region '%s'\n", volumesToReuse, action.GroupName, action.Region)
		case volumesToCreate > 0:
			fmt.Fprintf(io.Out, "%+4d volumes  for group '%s' in region '%s'\n", volumesToCreate, action.GroupName, action.Region)
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

	// XXX: Don't acquire the leases until the user confirms it wants to execute any action
	//      The downside is that AcquireLeases has the side effect of fetching an updated copy of machine config
	//      that we don't use here, but it also updates the `LeaseNonce` field of the original machine which we rely on
	_, releaseFunc, err := mach.AcquireLeases(ctx, machines)
	defer releaseFunc() // It's important to call the release func even in case of errors
	if err != nil {
		return err
	}

	updatePool := pool.New().
		WithErrors().
		WithMaxGoroutines(maxConcurrentActions).
		WithContext(ctx)

	fmt.Fprintf(io.Out, "Executing scale plan\n")
	for _, action := range actions {
		action := action
		switch {
		case action.Delta > 0:
			for i := 0; i < action.Delta; i++ {
				updatePool.Go(func(ctx context.Context) error {
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
					return nil
				})
			}
		case action.Delta < 0:
			for i := 0; i > action.Delta; i-- {
				updatePool.Go(func(ctx context.Context) error {
					m := action.Machines[-i]
					err := destroyMachine(ctx, m)
					if err != nil {
						return err
					}
					fmt.Fprintf(io.Out, "  Destroyed %s group:%s region:%s size:%s\n", m.ID, action.GroupName, action.Region, m.Config.Guest.ToSize())
					return nil
				})
			}
		}
	}

	return updatePool.Wait()
}

func launchMachine(ctx context.Context, action *planItem, idx int) (*fly.Machine, error) {
	flapsClient := flapsutil.ClientFromContext(ctx)
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	input := helpers.Clone(*action.LaunchMachineInput)

	if len(input.Config.Mounts) > 0 {
		var volume *fly.Volume

		switch {
		case idx < len(action.Volumes):
			volume = action.Volumes[idx]
		case action.CreateVolumeRequest != nil:
			cvr := action.CreateVolumeRequest
			fmt.Fprintf(io.Out, "  Creating volume %s region:%s", colorize.Bold(cvr.Name), cvr.Region)
			if cvr.SizeGb != nil {
				fmt.Fprintf(io.Out, " size:%dGiB", *cvr.SizeGb)
			}
			if cvr.SnapshotID != nil {
				fmt.Fprintf(io.Out, " from-snapshot:%s", colorize.Bold(*cvr.SnapshotID))
			}
			fmt.Fprintln(io.Out)

			var err error
			volume, err = flapsClient.CreateVolume(ctx, *cvr)
			if err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("Launching the machine requires a volume but there is no volume to attach or create")
		}
		input.Config.Mounts[0].Volume = volume.ID
	}

	return flapsClient.Launch(ctx, input)
}

func destroyMachine(ctx context.Context, machine *fly.Machine) error {
	flapsClient := flapsutil.ClientFromContext(ctx)
	input := fly.RemoveMachineInput{
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
	Machines           []*fly.Machine
	LaunchMachineInput *fly.LaunchMachineInput
	// Volumes to reuse
	Volumes []*fly.Volume
	// Input used to create new volumes
	CreateVolumeRequest *fly.CreateVolumeRequest
}

func (pi *planItem) VolumesDelta() int {
	if pi.CreateVolumeRequest == nil {
		return 0
	}
	return pi.Delta - len(pi.Volumes)
}

func (pi *planItem) MachineSize() string {
	if pi.Delta > 0 {
		return pi.LaunchMachineInput.Config.Guest.ToSize()
	}
	if len(pi.Machines) > 0 {
		return pi.Machines[0].Config.Guest.ToSize()
	}
	if guest := pi.LaunchMachineInput.Config.Guest; guest != nil {
		return guest.ToSize()
	}
	return ""
}

func computeActions(machines []*fly.Machine, expectedGroupCounts groupCounts, regions []string, maxPerRegion int, defaults *defaultValues) ([]*planItem, error) {
	actions := make([]*planItem, 0)
	seenGroups := make(map[string]bool)
	machineGroups := lo.GroupBy(machines, func(m *fly.Machine) string {
		return m.ProcessGroup()
	})
	expectedCounts := lo.MapValues(expectedGroupCounts, func(c groupCount, group string) int {
		count := c.absolute
		if c.relative != 0 {
			count = len(machineGroups[group]) + c.relative
		}
		return max(count, 0)
	})

	for groupName, groupMachines := range machineGroups {
		expected, ok := expectedCounts[groupName]
		// Ignore the group if it is not expected to change
		if !ok {
			continue
		}
		seenGroups[groupName] = true

		perRegionMachines := lo.GroupBy(groupMachines, func(m *fly.Machine) string {
			return m.Region
		})

		currentPerRegionCount := lo.MapEntries(perRegionMachines, func(k string, v []*fly.Machine) (string, int) {
			return k, len(v)
		})

		regionDiffs, err := convergeGroupCounts(expected, currentPerRegionCount, regions, maxPerRegion)
		if err != nil {
			return nil, err
		}

		mConfig := groupMachines[0].Config
		// Nullify standbys, no point on having more than one
		mConfig.Standbys = nil
		delete(mConfig.Env, "FLY_STANDBY_FOR")

		for region, delta := range regionDiffs {
			actions = append(actions, &planItem{
				GroupName:           groupName,
				Region:              region,
				Delta:               delta,
				Machines:            perRegionMachines[region],
				LaunchMachineInput:  &fly.LaunchMachineInput{Region: region, Config: mConfig},
				Volumes:             defaults.PopAvailableVolumes(mConfig, region, delta),
				CreateVolumeRequest: defaults.CreateVolumeRequest(mConfig, region, delta),
			})
		}
	}

	// Fill in the groups without existing machines
	for groupName, expected := range expectedCounts {
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
				LaunchMachineInput:  &fly.LaunchMachineInput{Region: region, Config: mConfig},
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
