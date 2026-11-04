package scale

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"testing"

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

func TestConvergeGroupCounts(t *testing.T) {
	for _, tc := range []struct {
		name    string
		count   groupCount
		current map[string]int
		regions []string
		cap     int
		want    map[string]int
		wantErr error
	}{
		{"zero", groupCount{}, map[string]int{"ord": 2, "sjc": 3}, []string{"ord"}, 0, map[string]int{"ord": -2}, nil},
		{"decrement clamps at zero", groupCount{relative: -3}, map[string]int{"ord": 2, "sjc": 4}, []string{"ord", "sjc", "ams"}, -1, map[string]int{"ord": -2, "sjc": -3}, nil},
		{"cap met", groupCount{absolute: 4}, map[string]int{"ord": 6, "sjc": 2}, []string{"ord", "sjc"}, 4, map[string]int{"ord": -2, "sjc": 2}, nil},
		{"absolute exceeds cap", groupCount{absolute: 4}, nil, []string{"ord", "sjc"}, 3, nil, ErrMaxPerRegion},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := convergeGroupCounts(tc.count, tc.current, tc.regions, tc.cap)
			require.ErrorIs(t, err, tc.wantErr)
			require.Equal(t, tc.want, got)
		})
	}
	t.Run("relative overflow cannot become a deletion", func(t *testing.T) {
		_, err := convergeGroupCounts(groupCount{relative: math.MaxInt}, map[string]int{"ord": 2}, []string{"ord"}, -1)
		require.ErrorContains(t, err, "overflows")
	})
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
	actions, err := computeActions("local-test", nil, groupCounts{"app": {absolute: 4}}, []string{"ord", "sjc", "ord"}, -1, defaults)
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
	machines []*fly.Machine
}

func (c *scaleCountReadClient) ListFlyAppsMachines(context.Context, string) ([]*fly.Machine, *fly.Machine, error) {
	return c.machines, nil, nil
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
	for _, tc := range []struct {
		name         string
		args         []string
		counts       map[string]map[string]int
		want         []string
		absent       []string
		wantCapError bool
		wantError    string
	}{
		{
			name:   "explicit regions and groups",
			args:   []string{"app=4", "worker=0", "-r", "ord,sjc,syd,lax,ams,ord"},
			counts: map[string]map[string]int{"app": {"ord": 6, "sjc": 1, "syd": 2, "lax": 2, "ams": 2, "nrt": 3}, "worker": {"ord": 2, "nrt": 3}, "untouched": {"ord": 2}},
			want:   []string{"-2 machines for group 'app' on region 'ord'", "+3 machines for group 'app' on region 'sjc'", "+2 machines for group 'app' on region 'syd'", "+2 machines for group 'app' on region 'lax'", "+2 machines for group 'app' on region 'ams'", "-2 machines for group 'worker' on region 'ord'"},
			absent: []string{"on region 'nrt'", "group 'untouched'", "+0 machines"},
		},
		{
			name:   "inferred regions belong to each group",
			args:   []string{"4"},
			counts: map[string]map[string]int{"app": {"ord": 2, "sjc": 4}, "worker": {"ams": 1}},
			want:   []string{"+2 machines for group 'app' on region 'ord'"},
			absent: []string{"on region 'sjc'"},
		},
		{
			name: "empty app uses primary region",
			args: []string{"4"},
			want: []string{"+4 machines for group 'app' on region 'ord'"},
		},
		{
			name:   "new group uses primary region",
			args:   []string{"4"},
			counts: map[string]map[string]int{"worker": {"ams": 1}},
			want:   []string{"+4 machines for group 'app' on region 'ord'"},
		},
		{
			name: "space padded regions",
			args: []string{"2", "-r", " ord, sjc "},
			want: []string{"+2 machines for group 'app' on region 'ord'", "+2 machines for group 'app' on region 'sjc'"},
		},
		{name: "empty region entry", args: []string{"2", "-r", "ord,"}, wantError: "region must not be empty"},
		{name: "empty region flag", args: []string{"2", "-r", ""}, wantError: "region must not be empty"},
		{
			name:   "relative count uses each selected region",
			args:   []string{"+1", "-r", "ord,sjc,ord"},
			counts: map[string]map[string]int{"app": {"ord": 2, "sjc": 4, "nrt": 8}},
			want:   []string{"+1 machines for group 'app' on region 'ord'", "+1 machines for group 'app' on region 'sjc'"},
			absent: []string{"on region 'nrt'"},
		},
		{
			name:         "cap rejects entire plan",
			args:         []string{"+1", "-r", "ord,sjc", "--max-per-region", "4"},
			counts:       map[string]map[string]int{"app": {"ord": 2, "sjc": 4}},
			wantCapError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newScaleCount()
			require.NoError(t, cmd.ParseFlags(tc.args))
			require.NoError(t, cmd.ValidateArgs(cmd.Flags().Args()))
			ctx := flag.NewContext(t.Context(), cmd.Flags())
			ctx = flapsutil.NewContextWithClient(ctx, &scaleCountReadClient{machines: countTestMachines(tc.counts)})
			ctx = uiexutil.NewContextWithClient(ctx, &scaleCountReleaseClient{})
			var out bytes.Buffer
			ctx = iostreams.NewContext(ctx, &iostreams.IOStreams{Out: &out, ErrOut: io.Discard})
			groups, err := parseGroupCounts(cmd.Flags().Args(), "app")
			require.NoError(t, err)
			err = runMachinesScaleCount(ctx, "local-test", &appconfig.Config{PrimaryRegion: "ord"}, groups, flag.GetInt(ctx, "max-per-region"))
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.Empty(t, out.String())
				return
			}
			if tc.wantCapError {
				require.ErrorIs(t, err, ErrMaxPerRegion)
				require.Empty(t, out.String())
				return
			}
			// Stop at confirmation: exercise production region selection and planning.
			require.ErrorContains(t, err, "--yes flag must be specified")
			for _, line := range tc.want {
				require.Contains(t, out.String(), line)
			}
			for _, line := range tc.absent {
				require.NotContains(t, out.String(), line)
			}
			require.Equal(t, len(tc.want), strings.Count(out.String(), " machines for group "))
		})
	}
}
