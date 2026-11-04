package scale

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/command/ips"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyerr"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/machinelimit"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

const maxConcurrentActions = 5

func runMachinesScaleCount(ctx context.Context, appName string, appConfig *appconfig.Config, expectedGroupCounts groupCounts, maxPerRegion int) error {
	io := iostreams.FromContext(ctx)
	flapsClient := flapsutil.ClientFromContext(ctx)
	ctx = appconfig.WithConfig(ctx, appConfig)
	uiexClient := uiexutil.ClientFromContext(ctx)

	machines, _, err := flapsClient.ListFlyAppsMachines(ctx, appName)
	if err != nil {
		return err
	}

	machines = lo.Filter(machines, func(m *fly.Machine, _ int) bool {
		return m.Config != nil
	})

	releases, err := uiexClient.ListReleases(ctx, appName, 25)
	if err != nil {
		return err
	}
	var latestCompleteRelease fly.Release
	if latest, ok := lo.Find(releases, func(r uiex.Release) bool { return r.Status == "complete" }); ok {
		latestCompleteRelease = fly.Release{ID: latest.ID, Version: latest.Version, ImageRef: latest.ImageRef}
	} else {
		return fmt.Errorf("this app has no complete releases. Run `fly deploy` to create one and rerun this command")
	}

	var regions []string
	if flag.IsSpecified(ctx, "region") {
		regions = strings.Split(flag.GetRegion(ctx), ",")
		for i, region := range regions {
			regions[i] = strings.TrimSpace(region)
			if regions[i] == "" {
				return fmt.Errorf("region must not be empty")
			}
		}
	}

	volumes, err := flapsClient.GetVolumes(ctx, appName)
	if err != nil {
		return err
	}

	defaultGuest, err := flag.GetMachineGuest(ctx, nil)
	if err != nil {
		return err
	}

	defaults := newDefaults(appConfig, latestCompleteRelease, machines, volumes,
		flag.GetString(ctx, "from-snapshot"), flag.GetBool(ctx, "with-new-volumes"), defaultGuest)

	actions, err := computeActions(appName, machines, expectedGroupCounts, regions, maxPerRegion, defaults)
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

		volumesToReuse := action.AvailableVolumeCount
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
	_, releaseFunc, err := mach.AcquireLeases(ctx, appName, machines)
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
		switch {
		case action.Delta > 0:
			for i := 0; i < action.Delta; i++ {
				updatePool.Go(func(ctx context.Context) error {
					m, err := launchMachine(ctx, appName, action, i)
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
					err := destroyMachine(ctx, appName, m)
					if err != nil {
						return err
					}
					fmt.Fprintf(io.Out, "  Destroyed %s group:%s region:%s size:%s\n", m.ID, action.GroupName, action.Region, m.Config.Guest.ToSize())

					return nil
				})
			}
		}
	}

	err = updatePool.Wait()
	if err != nil {
		err = withVolumePlacementCapacitySuggestion(err)
		if machinelimit.Refused(err) {
			err = machinelimit.WithRequestLink(ctx, err, machinelimit.AppOrgSlug(ctx, appName))
		}

		return err
	}

	// Scaling may change the situation of app-scoped egress IPs in affected regions
	regionMap := make(map[string]any, len(actions))
	for _, action := range actions {
		regionMap[action.Region] = nil
	}
	ips.SanityCheckAppScopedEgressIps(ctx, regionMap, nil, nil, "")

	// Scaling down alone never needs the warning.
	if lo.SomeBy(actions, func(a *planItem) bool { return a.Delta > 0 }) {
		machinelimit.Warn(ctx, machinelimit.AppOrgSlug(ctx, appName))
	}

	return nil
}

const volumePlacementCapacitySuggestion = "There isn't enough capacity on the hosts holding the available volumes to create the remaining Machines. " +
	"Try again later, or use --with-new-volumes to create new empty volumes on hosts with capacity."

func withVolumePlacementCapacitySuggestion(err error) error {
	if flapsutil.HasErrorStatusCode(err, flapsutil.VolumePlacementCapacityStatus) {
		return flyerr.WithSuggestion(err, volumePlacementCapacitySuggestion)
	}

	return err
}

func launchMachine(ctx context.Context, appName string, action *planItem, idx int) (*fly.Machine, error) {
	flapsClient := flapsutil.ClientFromContext(ctx)
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	input := helpers.Clone(*action.LaunchMachineInput)

	if len(input.Config.Mounts) > 0 {
		input.Config.Mounts[0].Volume = input.Config.Mounts[0].Name
		switch {
		case idx < action.AvailableVolumeCount:
			// Reuse an existing volume by name
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

			volume, err := flapsClient.CreateVolume(ctx, appName, *cvr)
			if err != nil {
				return nil, err
			}
			if action.WithNewVolumes {
				input.Config.Mounts[0].Volume = volume.ID
			}
		default:
			return nil, fmt.Errorf("Launching the machine requires a volume but there is no volume to attach or create")
		}
	}

	return flapsClient.Launch(ctx, appName, input)
}

func destroyMachine(ctx context.Context, appName string, machine *fly.Machine) error {
	flapsClient := flapsutil.ClientFromContext(ctx)
	input := fly.RemoveMachineInput{
		ID:   machine.ID,
		Kill: true,
	}

	return flapsClient.Destroy(ctx, appName, input, machine.LeaseNonce)
}

type planItem struct {
	GroupName string
	Region    string
	// The number of machines to add or remove
	Delta              int
	Machines           []*fly.Machine
	LaunchMachineInput *fly.LaunchMachineInput
	// Number of existing volumes to reuse by name
	AvailableVolumeCount int
	// Input used to create new volumes
	CreateVolumeRequest *fly.CreateVolumeRequest
	// Specifically attach newly created volumes by ID instead of selecting any by name
	WithNewVolumes bool
}

func (pi *planItem) VolumesDelta() int {
	if pi.CreateVolumeRequest == nil {
		return 0
	}

	return pi.Delta - pi.AvailableVolumeCount
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

func computeActions(appName string, machines []*fly.Machine, expectedGroupCounts groupCounts, regions []string, maxPerRegion int, defaults *defaultValues) ([]*planItem, error) {
	actions := make([]*planItem, 0)
	seenGroups := make(map[string]bool)
	machineGroups := lo.GroupBy(machines, func(m *fly.Machine) string {
		return m.ProcessGroup()
	})

	minvers, err := appsecrets.GetMinvers(appName)
	if err != nil {
		return nil, err
	}

	for groupName, groupMachines := range machineGroups {
		expected, ok := expectedGroupCounts[groupName]
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
			existingMachinesInRegion := perRegionMachines[region]
			actions = append(actions, &planItem{
				GroupName:            groupName,
				Region:               region,
				Delta:                delta,
				Machines:             existingMachinesInRegion,
				LaunchMachineInput:   &fly.LaunchMachineInput{Region: region, Config: mConfig, MinSecretsVersion: minvers},
				AvailableVolumeCount: defaults.consumeAvailableVolumeCount(mConfig, region, delta),
				CreateVolumeRequest:  defaults.CreateVolumeRequest(mConfig, region, delta, len(existingMachinesInRegion)),
				WithNewVolumes:       defaults.withNewVolumes,
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

		groupRegions := regions
		if len(groupRegions) == 0 {
			groupRegions = []string{defaults.appConfig.PrimaryRegion}
		}
		regionDiffs, err := convergeGroupCounts(expected, nil, groupRegions, maxPerRegion)
		if err != nil {
			return nil, err
		}

		for region, delta := range regionDiffs {
			actions = append(actions, &planItem{
				GroupName:            groupName,
				Region:               region,
				Delta:                delta,
				LaunchMachineInput:   &fly.LaunchMachineInput{Region: region, Config: mConfig, MinSecretsVersion: minvers},
				AvailableVolumeCount: defaults.consumeAvailableVolumeCount(mConfig, region, delta),
				CreateVolumeRequest:  defaults.CreateVolumeRequest(mConfig, region, delta, 0), // No existing machines for new groups
				WithNewVolumes:       defaults.withNewVolumes,
			})
		}
	}

	return actions, nil
}

var ErrMaxPerRegion = errors.New("requested count exceeds the maximum machines per region")

func convergeGroupCounts(count groupCount, current map[string]int, regions []string, maxPerRegion int) (map[string]int, error) {
	if len(regions) == 0 {
		regions = lo.Keys(current)
	}
	diffs := make(map[string]int)
	for _, region := range regions {
		expected := count.absolute
		if count.relative != 0 {
			if count.relative > 0 && current[region] > math.MaxInt-count.relative {
				return nil, fmt.Errorf("relative count overflows machine count in region %s", region)
			}
			expected = max(current[region]+count.relative, 0)
		}
		if maxPerRegion >= 0 && expected > maxPerRegion {
			return nil, ErrMaxPerRegion
		}
		if delta := expected - current[region]; delta != 0 {
			diffs[region] = delta
		}
	}
	return diffs, nil
}
