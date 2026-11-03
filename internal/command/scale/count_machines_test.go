package scale

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

type launchMachineFlapsClient struct {
	flapsutil.FlapsClient
	createRequests []fly.CreateVolumeRequest
	createdVolume  *fly.Volume
	createErr      error
	launchInput    fly.LaunchMachineInput
	launchCalls    int
	launchErr      error
}

func (c *launchMachineFlapsClient) CreateVolume(_ context.Context, _ string, req fly.CreateVolumeRequest) (*fly.Volume, error) {
	c.createRequests = append(c.createRequests, req)

	return c.createdVolume, c.createErr
}

func (c *launchMachineFlapsClient) Launch(_ context.Context, _ string, input fly.LaunchMachineInput) (*fly.Machine, error) {
	c.launchInput = input
	c.launchCalls++
	if c.launchErr != nil {
		return nil, c.launchErr
	}

	return &fly.Machine{Config: input.Config}, nil
}

func TestLaunchMachineVolumeSelection(t *testing.T) {
	newContext := func(client *launchMachineFlapsClient) context.Context {
		ctx := flapsutil.NewContextWithClient(context.Background(), client)

		return iostreams.NewContext(ctx, &iostreams.IOStreams{Out: io.Discard, ErrOut: io.Discard})
	}
	newAction := func() *planItem {
		return &planItem{
			LaunchMachineInput: &fly.LaunchMachineInput{
				Config: &fly.MachineConfig{
					Mounts: []fly.MachineMount{{Name: "data", Path: "/data"}},
				},
			},
		}
	}

	t.Run("existing volume is selected by name", func(t *testing.T) {
		client := &launchMachineFlapsClient{}
		action := newAction()
		action.AvailableVolumeCount = 1

		_, err := launchMachine(newContext(client), "app", action, 0)
		require.NoError(t, err)
		require.Empty(t, client.createRequests)
		require.Equal(t, "data", client.launchInput.Config.Mounts[0].Volume)
	})

	t.Run("newly created volume joins the named pool", func(t *testing.T) {
		client := &launchMachineFlapsClient{createdVolume: &fly.Volume{ID: "vol_created"}}
		action := newAction()
		action.CreateVolumeRequest = &fly.CreateVolumeRequest{Name: "data", Region: "qmx"}

		_, err := launchMachine(newContext(client), "app", action, 0)
		require.NoError(t, err)
		require.Len(t, client.createRequests, 1)
		require.Equal(t, "data", client.launchInput.Config.Mounts[0].Volume)
	})

	t.Run("with-new-volumes selects the newly created volume by ID", func(t *testing.T) {
		client := &launchMachineFlapsClient{createdVolume: &fly.Volume{ID: "vol_created"}}
		action := newAction()
		action.CreateVolumeRequest = &fly.CreateVolumeRequest{Name: "data", Region: "qmx"}
		action.WithNewVolumes = true

		_, err := launchMachine(newContext(client), "app", action, 0)
		require.NoError(t, err)
		require.Len(t, client.createRequests, 1)
		require.Equal(t, "vol_created", client.launchInput.Config.Mounts[0].Volume)
	})

	t.Run("missing volume source fails before launch", func(t *testing.T) {
		client := &launchMachineFlapsClient{}

		_, err := launchMachine(newContext(client), "app", newAction(), 0)
		require.ErrorContains(t, err, "there is no volume to attach or create")
		require.Zero(t, client.launchCalls)
	})

	t.Run("volume creation failure is returned before launch", func(t *testing.T) {
		client := &launchMachineFlapsClient{createErr: errors.New("volume create failed")}
		action := newAction()
		action.CreateVolumeRequest = &fly.CreateVolumeRequest{Name: "data", Region: "qmx"}

		_, err := launchMachine(newContext(client), "app", action, 0)
		require.ErrorIs(t, err, client.createErr)
		require.Len(t, client.createRequests, 1)
		require.Zero(t, client.launchCalls)
	})

	t.Run("launch failure after name selection is returned", func(t *testing.T) {
		client := &launchMachineFlapsClient{launchErr: errors.New("machine launch failed")}
		action := newAction()
		action.AvailableVolumeCount = 1

		_, err := launchMachine(newContext(client), "app", action, 0)
		require.ErrorIs(t, err, client.launchErr)
		require.Equal(t, 1, client.launchCalls)
		require.Equal(t, "data", client.launchInput.Config.Mounts[0].Volume)
	})
}

func TestVolumePlacementCapacitySuggestion(t *testing.T) {
	placementErr := &flaps.FlapsError{
		OriginalError: errors.New("failed_precondition: insufficient resources for volumes"),
		ResponseBody:  []byte(`{"status":"volume_placement_capacity"}`),
		FlyRequestId:  "request-id",
	}

	t.Run("adds scale advice and preserves Flaps metadata", func(t *testing.T) {
		err := withVolumePlacementCapacitySuggestion(fmt.Errorf("failed to launch VM: %w", placementErr))

		require.ErrorIs(t, err, placementErr)
		require.Equal(t, "request-id", flaps.GetErrorRequestID(err))
		require.Contains(t, flyerr.GetErrorSuggestion(err), "--with-new-volumes")
		require.Contains(t, flyerr.GetErrorSuggestion(err), "empty volumes")
	})

	t.Run("finds the status after another joined Flaps error", func(t *testing.T) {
		otherErr := &flaps.FlapsError{
			OriginalError: errors.New("another failure"),
			ResponseBody:  []byte(`{"status":"unknown"}`),
		}
		err := withVolumePlacementCapacitySuggestion(errors.Join(otherErr, placementErr))

		require.Contains(t, flyerr.GetErrorSuggestion(err), "--with-new-volumes")
	})

	t.Run("leaves unrelated errors unchanged", func(t *testing.T) {
		err := errors.New("machine launch failed")

		require.Same(t, err, withVolumePlacementCapacitySuggestion(err))
	})
}

func TestConsumeAvailableVolumeCount(t *testing.T) {
	defaults := &defaultValues{
		availableVolumeCounts: map[string]map[string]int{"data": {"qmx": 2}},
	}
	config := &fly.MachineConfig{Mounts: []fly.MachineMount{{Name: "data"}}}

	require.Equal(t, 2, defaults.consumeAvailableVolumeCount(config, "qmx", 3))
	require.Equal(t, 0, defaults.consumeAvailableVolumeCount(config, "qmx", 1))
	require.Equal(t, 0, (&defaultValues{}).consumeAvailableVolumeCount(config, "qmx", 1))
}

func TestNewDefaultsOnlyCountsReusableVolumes(t *testing.T) {
	attachedMachine := "machine-id"
	attachedAllocation := "allocation-id"
	volumes := []fly.Volume{
		{Name: "data", Region: "qmx", State: "created", HostStatus: "ok"},
		{Name: "data", Region: "qmx", State: "created", HostStatus: "ok"},
		{Name: "data", Region: "syd", State: "created", HostStatus: "ok"},
		{Name: "data", Region: "qmx", State: "created", HostStatus: "unreachable"},
		{Name: "data", Region: "qmx", State: "created", HostStatus: "ok", AttachedMachine: &attachedMachine},
		{Name: "data", Region: "qmx", State: "created", HostStatus: "ok", AttachedAllocation: &attachedAllocation},
		{Name: "other", Region: "qmx", State: "created", HostStatus: "ok"},
	}

	defaults := newDefaults(&appconfig.Config{}, fly.Release{}, nil, volumes, "", false, nil)
	require.Equal(t, map[string]map[string]int{
		"data":  {"qmx": 2, "syd": 1},
		"other": {"qmx": 1},
	}, defaults.availableVolumeCounts)

	createOnly := newDefaults(&appconfig.Config{}, fly.Release{}, nil, volumes, "", true, nil)
	require.Nil(t, createOnly.availableVolumeCounts)
	require.True(t, createOnly.withNewVolumes)
}

func Test_convergeGroupCounts(t *testing.T) {
	testcases := []struct {
		name          string
		want          map[string]int
		expectedTotal int
		current       map[string]int
		regions       []string
		maxPerRegion  int
	}{
		{
			name:          "Spread instances across regions from nothing",
			want:          map[string]int{"scl": 2, "iad": 1},
			expectedTotal: 3,
			regions:       []string{"scl", "iad"},
			maxPerRegion:  -1,
		},
		{
			name:          "Spread instances across regions from existing",
			want:          map[string]int{"scl": 1},
			current:       map[string]int{"scl": 1, "iad": 1},
			expectedTotal: 3,
			regions:       []string{"scl", "iad"},
			maxPerRegion:  -1,
		},
		{
			name:          "Act on all current regions if not region is passed",
			want:          map[string]int{"scl": 2, "iad": 2},
			current:       map[string]int{"scl": 1, "iad": 1},
			expectedTotal: 6,
			maxPerRegion:  -1,
		},
		{
			name:          "Requirements already met",
			want:          map[string]int{},
			current:       map[string]int{"scl": 1, "iad": 1},
			expectedTotal: 2,
			regions:       []string{"scl", "iad"},
			maxPerRegion:  -1,
		},
		{
			name:          "Reduce the fleet",
			want:          map[string]int{"iad": -1},
			current:       map[string]int{"scl": 1, "iad": 1},
			expectedTotal: 1,
			regions:       []string{"scl", "iad"},
			maxPerRegion:  -1,
		},
		{
			name:          "Reduce the fleet (like previous but order of regions matters)",
			want:          map[string]int{"scl": -1},
			current:       map[string]int{"scl": 1, "iad": 1},
			expectedTotal: 1,
			regions:       []string{"iad", "scl"},
			maxPerRegion:  -1,
		},
		// Ignore non-listed regions
		{
			name:          "Ignore non-listed regions while removing machines",
			want:          map[string]int{"scl": -3, "iad": -3},
			current:       map[string]int{"scl": 3, "iad": 5, "ord": 1, "sin": 10},
			expectedTotal: 2,
			regions:       []string{"scl", "iad"},
			maxPerRegion:  -1,
		},
		{
			name:          "Ignore non-listed regions while adding machines",
			want:          map[string]int{"scl": 2, "iad": 2},
			current:       map[string]int{"scl": 3, "iad": 5, "ord": 1, "sin": 10},
			expectedTotal: 12,
			regions:       []string{"scl", "iad"},
			maxPerRegion:  -1,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := convergeGroupCounts(tc.expectedTotal, tc.current, tc.regions, tc.maxPerRegion)
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func Test_convergeGroupCounts_maxPerRegion(t *testing.T) {
	// maxPerRegion * len(regions) < expectedTotal must fail
	_, err := convergeGroupCounts(10, nil, []string{"scl", "mia"}, 1)
	assert.Equal(t, ErrMaxPerRegion, err)

	// Happy path cases
	testcases := []struct {
		name          string
		want          map[string]int
		expectedTotal int
		current       map[string]int
		regions       []string
		maxPerRegion  int
	}{
		{
			name:          "Spread instances across regions respecting max per region",
			want:          map[string]int{"scl": 1, "iad": 2},
			current:       map[string]int{"scl": 2, "iad": 1},
			expectedTotal: 6,
			regions:       []string{"scl", "iad"},
			maxPerRegion:  3,
		},
		{
			name:          "Spread instances across regions respecting max per regioni with reductions",
			want:          map[string]int{"scl": -5, "iad": 1, "ord": 2, "sin": 2},
			current:       map[string]int{"scl": 7, "iad": 1},
			expectedTotal: 8,
			regions:       []string{"scl", "iad", "ord", "sin"},
			maxPerRegion:  2,
		},
		{
			name:          "Spread respecting unlisted regions",
			want:          map[string]int{"scl": -5, "iad": 1, "ord": 2, "sin": 2},
			current:       map[string]int{"scl": 7, "iad": 1, "mia": 10},
			expectedTotal: 8,
			regions:       []string{"scl", "iad", "ord", "sin"},
			maxPerRegion:  2,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := convergeGroupCounts(tc.expectedTotal, tc.current, tc.regions, tc.maxPerRegion)
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func Test_convergeGroupCounts_duplicateRegions(t *testing.T) {
	errCh := make(chan error, 1)
	go func() {
		// Pass a duplicate region. This is a regression test because the function
		// would choke on duplicates.
		_, err := convergeGroupCounts(20, nil, []string{"dfw", "sjc", "lhr", "lax", "cdg", "ams", "dfw", "gru", "arn", "sin"}, 2)
		errCh <- err
	}()

	select {
	case err := <-errCh:
		assert.ErrorIs(t, err, ErrMaxPerRegion)
	case <-time.After(time.Second):
		t.Fatal("convergeGroupCounts did not return when regions contained a duplicate")
	}
}

func countTestMachines(counts map[string]map[string]int) []*fly.Machine {
	var machines []*fly.Machine
	for group, regions := range counts {
		for region, count := range regions {
			for i := 0; i < count; i++ {
				machines = append(machines, &fly.Machine{Region: region, Config: &fly.MachineConfig{
					Metadata: map[string]string{fly.MachineConfigMetadataKeyFlyProcessGroup: group},
					Guest:    &fly.MachineGuest{CPUKind: "shared", CPUs: 1, MemoryMB: 256},
				}})
			}
		}
	}

	return machines
}

func TestComputeActionsPerRegionNewGroup(t *testing.T) {
	defaults := newDefaults(&appconfig.Config{PrimaryRegion: "ord"}, fly.Release{ImageRef: "test-image"}, nil, nil, "", false, nil)
	actions, err := computeActions("local-test", nil, groupCounts{"app": {absolute: 4}}, []string{"ord", "sjc", "ord"}, -1, defaults, true)
	require.NoError(t, err)
	require.Len(t, actions, 2)
	for _, action := range actions {
		require.Equal(t, 4, action.Delta)
		require.Contains(t, []string{"ord", "sjc"}, action.Region)
		require.Equal(t, action.Region, action.LaunchMachineInput.Region)
	}
}

// Unimplemented client methods panic, so this fixture cannot execute mutations.
type scaleCountReadClient struct {
	flapsutil.FlapsClient
}

func (*scaleCountReadClient) ListFlyAppsMachines(context.Context, string) ([]*fly.Machine, *fly.Machine, error) {
	return countTestMachines(map[string]map[string]int{"app": {"ord": 2, "sjc": 2, "ams": 4}}), nil, nil
}

func (*scaleCountReadClient) GetVolumes(context.Context, string) ([]fly.Volume, error) {
	return nil, nil
}

type scaleCountReleaseClient struct {
	uiexutil.Client
}

func (*scaleCountReleaseClient) ListReleases(context.Context, string, int) ([]uiex.Release, error) {
	return []uiex.Release{{Status: "complete", ImageRef: "test-image"}}, nil
}

func TestRunMachinesScaleCountPerRegion(t *testing.T) {
	cmd := newScaleCount()
	require.NoError(t, cmd.ParseFlags([]string{"4", "--per-region", "-r", "ord,sjc,ams"}))
	require.NoError(t, cmd.ValidateArgs(cmd.Flags().Args()))
	ctx := flag.NewContext(t.Context(), cmd.Flags())
	ctx = flapsutil.NewContextWithClient(ctx, &scaleCountReadClient{})
	ctx = uiexutil.NewContextWithClient(ctx, &scaleCountReleaseClient{})
	var out bytes.Buffer
	ctx = iostreams.NewContext(ctx, &iostreams.IOStreams{Out: &out, ErrOut: io.Discard})
	groups, err := parseGroupCounts(cmd.Flags().Args(), "app")
	require.NoError(t, err)
	err = runMachinesScaleCount(ctx, "local-test", &appconfig.Config{}, groups, -1)
	// Stop at confirmation: exercise the production flag read and plan, not execution.
	require.ErrorContains(t, err, "--yes flag must be specified")
	require.Contains(t, out.String(), "+2 machines for group 'app' on region 'ord'")
	require.Contains(t, out.String(), "+2 machines for group 'app' on region 'sjc'")
	require.NotContains(t, out.String(), "on region 'ams'")
}
