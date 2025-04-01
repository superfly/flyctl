package deploy

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/statuslogger"
	"github.com/superfly/flyctl/iostreams"
)

func TestCompareConfig(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Test that two identical configs are equal
	config1 := &fly.MachineConfig{
		Image: "image1",
		Metadata: map[string]string{
			"fly_flyctl_version": "v1",
		},
	}
	config2 := &fly.MachineConfig{
		Image: "image1",
		Metadata: map[string]string{
			"fly_flyctl_version": "v1",
		},
	}

	assert.True(t, compareConfigs(ctx, config1, config2))

	// Test that fly_flyctl_version is ignored
	config2.Metadata["fly_flyctl_version"] = "v2"
	assert.True(t, compareConfigs(ctx, config1, config2))

	// Test that different images are not equal
	config2.Image = "image2"
	assert.False(t, compareConfigs(ctx, config1, config2))
}

func TestAppState(t *testing.T) {
	t.Parallel()

	machines := []*fly.Machine{
		{
			ID:    "machine1",
			State: "started",
		},
		{
			ID:    "machine2",
			State: "started",
		},
	}

	flapsClient := &mock.FlapsClient{
		ListFunc: func(ctx context.Context, state string) ([]*fly.Machine, error) {
			return machines, nil
		},
	}

	ctx := context.Background()
	md := &machineDeployment{
		flapsClient: flapsClient,
	}
	appState, error := md.appState(ctx, nil)
	assert.NoError(t, error)

	assert.Equal(t, appState.Machines, machines)

}

func TestUpdateMachineConfig(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = withQuietIOStreams(ctx)

	oldMachine := &fly.Machine{
		ID:         "machine1",
		HostStatus: fly.HostStatusOk,
		Config: &fly.MachineConfig{
			Image: "image1",
			Metadata: map[string]string{
				"fly_flyctl_version": "v1",
			},
		},
		LeaseNonce: "hi",
	}
	newMachineConfig := &fly.MachineConfig{
		Image: "image1",
		Metadata: map[string]string{
			"fly_flyctl_version": "v1",
		},
	}

	statuslogger := statuslogger.Create(ctx, 1, false)
	sl := statuslogger.Line(0)

	badFlapsClient := &mock.FlapsClient{
		UpdateFunc: func(ctx context.Context, builder fly.LaunchMachineInput, nonce string) (out *fly.Machine, err error) {
			return nil, assert.AnError
		},
	}
	md := &machineDeployment{
		flapsClient: badFlapsClient,
		io:          iostreams.FromContext(ctx),
		app: &fly.AppCompact{
			Name: "myapp",
		},
		appConfig: &appconfig.Config{AppName: "myapp"},
	}
	_, err := md.updateMachineConfig(ctx, oldMachine, newMachineConfig, sl, false)
	// updating a config with the same config should result in a noop, and shouldn't even use flaps
	assert.NoError(t, err)

	destroyedMachine := false
	// recreate machines when we specify that
	flapsClient := &mock.FlapsClient{
		DestroyFunc: func(ctx context.Context, input fly.RemoveMachineInput, nonce string) (err error) {
			destroyedMachine = true
			return nil
		},
		UpdateFunc: func(ctx context.Context, builder fly.LaunchMachineInput, nonce string) (out *fly.Machine, err error) {
			return &fly.Machine{
				ID:         builder.ID,
				HostStatus: fly.HostStatusOk,
				Config:     builder.Config,
			}, nil
		},
		LaunchFunc: func(ctx context.Context, builder fly.LaunchMachineInput) (out *fly.Machine, err error) {
			return &fly.Machine{
				ID:         builder.ID,
				HostStatus: fly.HostStatusOk,
				Config:     builder.Config,
			}, nil
		},

		ReleaseLeaseFunc: func(ctx context.Context, machineID, nonce string) error {
			return nil
		},
	}

	newMachineConfig.Image = "image2"
	md.flapsClient = flapsClient

	// ensure that we're actually creating a new machine when we replace it
	machine, err := md.updateMachineConfig(ctx, oldMachine, newMachineConfig, sl, true)
	assert.NoError(t, err)
	assert.Equal(t, machine.Config, newMachineConfig)
	assert.NotEqual(t, machine.ID, "machine2")
	assert.True(t, destroyedMachine)

	// just a regular machine update
	machine, err = md.updateMachineConfig(ctx, oldMachine, newMachineConfig, sl, false)
	assert.NoError(t, err)
	assert.NotNil(t, machine)
	assert.Equal(t, machine.Config, newMachineConfig)
}

func withQuietIOStreams(ctx context.Context) context.Context {
	ios, _, _, _ := iostreams.Test()
	return iostreams.NewContext(ctx, ios)
}

func TestUpdateMachines(t *testing.T) {
	t.Parallel()

	ctx := withQuietIOStreams(context.Background())

	oldMachines := []*fly.Machine{
		{
			ID:         "machine1",
			State:      "started",
			HostStatus: fly.HostStatusOk,
			Config: &fly.MachineConfig{
				Image: "image1",
			},
		},
		{
			ID:         "machine2",
			State:      "started",
			HostStatus: fly.HostStatusOk,
			Config: &fly.MachineConfig{
				Image: "image1",
			},
		},
		{
			ID:         "machine3",
			State:      "started",
			HostStatus: fly.HostStatusUnreachable,
			IncompleteConfig: &fly.MachineConfig{
				Image: "image1",
			},
		},
	}
	newMachines := lo.Map(oldMachines, func(m *fly.Machine, _ int) *fly.Machine {
		return &fly.Machine{
			ID:         m.ID,
			State:      "started",
			HostStatus: fly.HostStatusOk,
			Config: &fly.MachineConfig{
				Image: "image2",
			},
		}
	})

	acquiredLeases := sync.Map{}

	flapsClient := &mock.FlapsClient{
		AcquireLeaseFunc: func(ctx context.Context, machineID string, ttl *int) (*fly.MachineLease, error) {
			if _, ok := acquiredLeases.Load(machineID); ok {
				return nil, assert.AnError
			}
			acquiredLeases.Store(machineID, true)
			return &fly.MachineLease{
				Data: &fly.MachineLeaseData{
					Nonce: machineID + "nonce",
				},
			}, nil
		},
		ReleaseLeaseFunc: func(ctx context.Context, machineID, nonce string) error {
			return nil
		},
		UpdateFunc: func(ctx context.Context, builder fly.LaunchMachineInput, nonce string) (out *fly.Machine, err error) {
			return &fly.Machine{
				ID:         builder.ID,
				Config:     builder.Config,
				State:      "started",
				HostStatus: fly.HostStatusOk,
			}, nil
		},
		LaunchFunc: func(ctx context.Context, builder fly.LaunchMachineInput) (out *fly.Machine, err error) {
			return &fly.Machine{
				ID:         builder.ID,
				Config:     builder.Config,
				State:      "started",
				HostStatus: fly.HostStatusOk,
			}, nil
		},
		DestroyFunc: func(ctx context.Context, input fly.RemoveMachineInput, nonce string) (err error) {
			return nil
		},
		WaitFunc: func(ctx context.Context, machine *fly.Machine, state string, timeout time.Duration) (err error) {
			if state == "started" {
				machine.State = "started"
				return nil
			} else {
				return assert.AnError
			}
		},
		ListFunc: func(ctx context.Context, state string) ([]*fly.Machine, error) {
			return oldMachines, nil
		},
		StartFunc: func(ctx context.Context, machineID string, nonce string) (out *fly.MachineStartResponse, err error) {
			return &fly.MachineStartResponse{}, nil
		},
		GetFunc: func(ctx context.Context, machineID string) (*fly.Machine, error) {
			newMachine, _ := lo.Find(newMachines, func(m *fly.Machine) bool {
				return m.ID == machineID
			})
			return newMachine, nil
		},
		GetProcessesFunc: func(ctx context.Context, machineID string) (fly.MachinePsResponse, error) {
			return fly.MachinePsResponse{}, nil
		},
		RefreshLeaseFunc: func(ctx context.Context, machineID string, ttl *int, nonce string) (*fly.MachineLease, error) {
			return &fly.MachineLease{
				Status: "success",
				Data: &fly.MachineLeaseData{
					Nonce: nonce,
				},
			}, nil
		},
	}

	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)
	md := &machineDeployment{
		flapsClient: flapsClient,
		io:          iostreams.FromContext(ctx),
		app: &fly.AppCompact{
			Name: "myapp",
		},
		appConfig:      &appconfig.Config{AppName: "myapp"},
		waitTimeout:    10 * time.Second,
		deployRetries:  5,
		maxUnavailable: 3,
	}

	oldAppState := &AppState{
		Machines: oldMachines,
	}
	newAppState := &AppState{
		Machines: newMachines,
	}
	settings := updateMachineSettings{
		pushForward:          true,
		skipHealthChecks:     false,
		skipSmokeChecks:      false,
		skipLeaseAcquisition: false,
	}

	acquiredLeases = sync.Map{}
	err := md.updateMachinesWRecovery(ctx, oldAppState, newAppState, nil, settings)
	assert.NoError(t, err)

	// let's make sure we retry deploys a few times
	numFailures := 0
	maxNumFailures := 3
	flapsClient.UpdateFunc = func(ctx context.Context, builder fly.LaunchMachineInput, nonce string) (out *fly.Machine, err error) {
		if builder.ID == "machine2" {
			numFailures++
			if numFailures < maxNumFailures {
				return nil, assert.AnError
			}
		}

		return &fly.Machine{
			ID:         builder.ID,
			Config:     builder.Config,
			State:      "started",
			HostStatus: fly.HostStatusOk,
		}, nil
	}
	acquiredLeases = sync.Map{}
	err = md.updateMachinesWRecovery(ctx, oldAppState, newAppState, nil, settings)
	assert.NoError(t, err)
	assert.Equal(t, 3, numFailures)

	numFailures = 0
	maxNumFailures = 10
	acquiredLeases = sync.Map{}
	err = md.updateMachinesWRecovery(ctx, oldAppState, newAppState, nil, settings)
	assert.Error(t, err)

	var sentUnrecoverable atomic.Bool
	// we only return a single unrecoverable error, but that's enough to fail the deploy
	flapsClient.UpdateFunc = func(ctx context.Context, builder fly.LaunchMachineInput, nonce string) (out *fly.Machine, err error) {
		if !sentUnrecoverable.Load() {
			sentUnrecoverable.Store(true)
			return nil, &unrecoverableError{err: assert.AnError}
		} else {
			return &fly.Machine{
				ID:         builder.ID,
				Config:     builder.Config,
				State:      "started",
				HostStatus: fly.HostStatusOk,
			}, nil
		}
	}
	acquiredLeases = sync.Map{}
	err = md.updateMachinesWRecovery(ctx, oldAppState, newAppState, nil, settings)
	assert.Error(t, err)
	var unrecoverableErr *unrecoverableError
	assert.ErrorAs(t, err, &unrecoverableErr)
}

func TestUpdateOrCreateMachine(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = withQuietIOStreams(ctx)

	destroyedMachine := false
	updatedMachine := false
	createMachine := false

	reset := func() {
		destroyedMachine = false
		updatedMachine = false
		createMachine = false
	}

	flapsClient := &mock.FlapsClient{
		AcquireLeaseFunc: func(ctx context.Context, machineID string, ttl *int) (*fly.MachineLease, error) {
			return &fly.MachineLease{
				Data: &fly.MachineLeaseData{
					Nonce: "nonce",
				},
			}, nil
		},
		DestroyFunc: func(ctx context.Context, input fly.RemoveMachineInput, nonce string) (err error) {
			destroyedMachine = true
			return nil
		},
		UpdateFunc: func(ctx context.Context, builder fly.LaunchMachineInput, nonce string) (out *fly.Machine, err error) {
			updatedMachine = true
			return &fly.Machine{
				ID:         builder.ID,
				Config:     builder.Config,
				State:      "started",
				HostStatus: fly.HostStatusOk,
				LeaseNonce: "nonce",
			}, nil
		},
		LaunchFunc: func(ctx context.Context, builder fly.LaunchMachineInput) (out *fly.Machine, err error) {
			createMachine = true
			return &fly.Machine{
				ID:         builder.ID,
				Config:     builder.Config,
				State:      "started",
				HostStatus: fly.HostStatusOk,
				LeaseNonce: "nonce",
			}, nil
		},
	}

	oldMachine := &fly.Machine{
		ID:         "machine1",
		HostStatus: fly.HostStatusOk,
		Config: &fly.MachineConfig{
			Image: "image1",
		},
		LeaseNonce: "nonce",
	}
	newMachine := &fly.Machine{
		ID:         "machine1",
		HostStatus: fly.HostStatusOk,
		Config: &fly.MachineConfig{
			Image: "image2",
		},
		LeaseNonce: "nonce",
	}

	md := &machineDeployment{
		flapsClient: flapsClient,
		io:          iostreams.FromContext(ctx),
		app: &fly.AppCompact{
			Name: "myapp",
		},
		appConfig: &appconfig.Config{AppName: "myapp"},
	}

	statuslogger := statuslogger.Create(ctx, 1, false)
	sl := statuslogger.Line(0)

	// destroy old machine
	reset()
	mach, lease, err := md.updateOrCreateMachine(ctx, oldMachine, nil, sl)
	assert.NoError(t, err)
	assert.True(t, destroyedMachine)
	assert.Nil(t, mach)
	assert.Nil(t, lease)

	// update old machine
	reset()
	mach, lease, err = md.updateOrCreateMachine(ctx, oldMachine, newMachine, sl)
	assert.NoError(t, err)
	assert.True(t, updatedMachine)
	assert.NotNil(t, mach)
	assert.Nil(t, lease)
	assert.Equal(t, mach.Config, newMachine.Config)

	// create new machine
	reset()
	mach, lease, err = md.updateOrCreateMachine(ctx, nil, newMachine, sl)
	assert.NoError(t, err)
	assert.True(t, createMachine)
	assert.NotNil(t, mach)
	assert.NotNil(t, lease)
	assert.Equal(t, mach.Config, newMachine.Config)

	// new and old machines are nil, so noop
	reset()
	mach, lease, err = md.updateOrCreateMachine(ctx, nil, nil, sl)
	assert.NoError(t, err)
	assert.False(t, destroyedMachine)
	assert.False(t, updatedMachine)
	assert.False(t, createMachine)
	assert.Nil(t, mach)
	assert.Nil(t, lease)

}
