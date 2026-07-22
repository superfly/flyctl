package deploy

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

func TestNew(t *testing.T) {
	strategy := BlueGreenStrategy(&machineDeployment{}, nil)
	assert.False(t, strategy.isAborted())
}

func newBlueGreenStrategy(client flapsutil.FlapsClient, numberOfExistingMachines int) *blueGreen {
	var machines []*machineUpdateEntry
	ios, _, _, _ := iostreams.Test()

	for range numberOfExistingMachines {
		machines = append(machines, &machineUpdateEntry{
			leasableMachine: machine.NewLeasableMachine(client, ios, "", &fly.Machine{}, false),
			launchInput: &fly.LaunchMachineInput{
				Config: &fly.MachineConfig{
					Metadata: map[string]string{},
					Checks: map[string]fly.MachineCheck{
						"check1": {},
					},
				},
				MinSecretsVersion: nil,
			},
		})
	}
	strategy := &blueGreen{
		apiClient:       &mockWebClient{},
		flaps:           client,
		maxConcurrent:   10,
		appConfig:       &appconfig.Config{},
		io:              ios,
		colorize:        ios.ColorScheme(),
		clearLinesAbove: func(int) {}, // no-op; avoids nil-panic in render loop
		timeout:         5 * time.Second,
		blueMachines:    machines,
		app:             &flaps.App{Name: "test-app"},
	}
	strategy.initialize()

	// Don't have to wait during tests.
	strategy.waitBeforeStop = 0
	strategy.waitBeforeCordon = 0
	strategy.uncordonRetryDelay = 0

	return strategy
}

func TestDeploy(t *testing.T) {
	flapsClient := &mockFlapsClient{}

	ctx := context.Background()

	// Some functions take a client from the context.
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	// Happy cases
	t.Run("replace 1 machine", func(t *testing.T) {
		flapsClient.breakLaunch = false
		strategy := newBlueGreenStrategy(flapsClient, 1)

		err := strategy.Deploy(ctx)
		assert.NoError(t, err)
	})
	t.Run("replace 10 machine", func(t *testing.T) {
		flapsClient.breakLaunch = false
		strategy := newBlueGreenStrategy(flapsClient, 10)

		err := strategy.Deploy(ctx)
		assert.NoError(t, err)
	})

	// Error cases
	t.Run("no existing machines", func(t *testing.T) {
		strategy := newBlueGreenStrategy(flapsClient, 0)

		err := strategy.Deploy(ctx)
		assert.ErrorContains(t, err, "found multiple image versions")
	})
	t.Run("failed to launch machines", func(t *testing.T) {
		flapsClient.breakLaunch = true
		strategy := newBlueGreenStrategy(flapsClient, 1)

		err := strategy.Deploy(ctx)
		assert.ErrorContains(t, err, "failed to create green machines")
	})
}

func TestMarkGreenMachinesAsReadyForTrafficRetries(t *testing.T) {
	ios, _, _, _ := iostreams.Test()

	// makeStrategyWithGreenMachines builds a blueGreen with pre-populated green
	// machines, letting us test MarkGreenMachinesAsReadyForTraffic in isolation
	// without running the full deploy pipeline.
	makeStrategyWithGreenMachines := func(client *mockFlapsClient, greenCount int) *blueGreen {
		bg := newBlueGreenStrategy(client, 0)
		for i := range greenCount {
			bg.greenMachines = append(bg.greenMachines, &machineUpdateEntry{
				leasableMachine: machine.NewLeasableMachine(client, ios, "test-app", &fly.Machine{ID: fmt.Sprintf("green-%d", i+1)}, false),
				launchInput:     &fly.LaunchMachineInput{},
			})
		}

		return bg
	}

	ctx := context.Background()

	t.Run("succeeds immediately when no errors occur", func(t *testing.T) {
		client := &mockFlapsClient{}
		bg := makeStrategyWithGreenMachines(client, 3)

		err := bg.MarkGreenMachinesAsReadyForTraffic(ctx)
		assert.NoError(t, err)
	})

	t.Run("succeeds after transient uncordon failures are retried", func(t *testing.T) {
		client := &mockFlapsClient{uncordonTransientFailures: 2}
		bg := makeStrategyWithGreenMachines(client, 1)

		err := bg.MarkGreenMachinesAsReadyForTraffic(ctx)
		assert.NoError(t, err)

		client.mu.Lock()
		remaining := client.uncordonTransientFailures
		client.mu.Unlock()
		assert.Equal(t, 0, remaining, "all transient failures should have been consumed by retries")
	})

	t.Run("fails after all retry attempts are exhausted", func(t *testing.T) {
		client := &mockFlapsClient{breakUncordon: true}
		bg := makeStrategyWithGreenMachines(client, 1)
		bg.uncordonRetryAttempts = 3

		err := bg.MarkGreenMachinesAsReadyForTraffic(ctx)
		assert.ErrorContains(t, err, "failed to uncordon")
	})
}

func FuzzDeploy(f *testing.F) {
	flapsClient := &mockFlapsClient{}

	ctx := context.Background()

	// Some functions take a client from the context.
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	f.Add(20, false, false, false, false)

	f.Fuzz(func(t *testing.T, numberOfExistingMachines int, breakLaunch bool, breakWait bool, breakUncordon bool, breakSetMetadata bool) {
		strategy := newBlueGreenStrategy(flapsClient, numberOfExistingMachines)
		flapsClient.breakLaunch = breakLaunch
		flapsClient.breakWait = breakWait
		flapsClient.breakUncordon = breakUncordon
		flapsClient.breakSetMetadata = breakSetMetadata

		// At least, Deploy must not panic.
		strategy.Deploy(ctx)
	})
}

// ---------------------------------------------------------------------------
// Tests for the SkipLaunch / health-check fixes
// ---------------------------------------------------------------------------

// TestMachineHasConfiguredChecks verifies the helper that decides whether a
// machine config carries any health-check definitions.
func TestMachineHasConfiguredChecks(t *testing.T) {
	t.Run("no checks at all", func(t *testing.T) {
		cfg := &fly.MachineConfig{}
		assert.False(t, machineHasConfiguredChecks(cfg))
	})

	t.Run("top-level check", func(t *testing.T) {
		cfg := &fly.MachineConfig{
			Checks: map[string]fly.MachineCheck{"alive": {}},
		}
		assert.True(t, machineHasConfiguredChecks(cfg))
	})

	t.Run("service-level check only", func(t *testing.T) {
		cfg := &fly.MachineConfig{
			Services: []fly.MachineService{
				{Checks: []fly.MachineServiceCheck{{}}},
			},
		}
		assert.True(t, machineHasConfiguredChecks(cfg))
	})

	t.Run("service with no checks", func(t *testing.T) {
		cfg := &fly.MachineConfig{
			Services: []fly.MachineService{
				{Checks: nil},
			},
		}
		assert.False(t, machineHasConfiguredChecks(cfg))
	})
}

// newBlueGreenStrategyWithState is like newBlueGreenStrategy but lets the
// caller specify the state of each blue machine and its SkipLaunch value.
// This is used to simulate machines that have been auto-stopped.
func newBlueGreenStrategyWithState(client flapsutil.FlapsClient, machineState string, skipLaunch bool) *blueGreen {
	ios, _, _, _ := iostreams.Test()

	machines := []*machineUpdateEntry{
		{
			leasableMachine: machine.NewLeasableMachine(client, ios, "", &fly.Machine{
				State: machineState,
				Config: &fly.MachineConfig{
					Metadata: map[string]string{},
					Checks: map[string]fly.MachineCheck{
						"check1": {},
					},
				},
			}, false),
			launchInput: &fly.LaunchMachineInput{
				SkipLaunch: skipLaunch,
				Config: &fly.MachineConfig{
					Metadata: map[string]string{},
					Checks: map[string]fly.MachineCheck{
						"check1": {},
					},
				},
				MinSecretsVersion: nil,
			},
		},
	}

	strategy := &blueGreen{
		apiClient:       &mockWebClient{},
		flaps:           client,
		maxConcurrent:   10,
		appConfig:       &appconfig.Config{},
		io:              ios,
		colorize:        ios.ColorScheme(),
		clearLinesAbove: func(int) {},
		timeout:         5 * time.Second,
		blueMachines:    machines,
		app:             &flaps.App{Name: "test-app"},
	}
	strategy.initialize()
	strategy.waitBeforeStop = 0
	strategy.waitBeforeCordon = 0
	strategy.uncordonRetryDelay = 0

	return strategy
}

// TestCreateGreenMachinesAlwaysStartsGreenMachines verifies that green
// machines are always launched with SkipLaunch=false, even when the
// corresponding blue machine has SkipLaunch=true (e.g. because it was
// auto-stopped before the deploy).
func TestCreateGreenMachinesAlwaysStartsGreenMachines(t *testing.T) {
	client := &mockFlapsClient{}
	ctx := context.Background()
	ctx = flapsutil.NewContextWithClient(ctx, client)

	// Simulate a stopped blue machine: SkipLaunch=true.
	strategy := newBlueGreenStrategyWithState(client, fly.MachineStateStopped, true)

	err := strategy.CreateGreenMachines(ctx)
	assert.NoError(t, err)
	assert.Len(t, strategy.greenMachines, 1, "expected one green machine to be created")

	client.mu.Lock()
	inputs := client.launchInputs
	client.mu.Unlock()

	assert.Len(t, inputs, 1, "expected one Launch call")
	assert.False(t, inputs[0].SkipLaunch,
		"green machine must be launched with SkipLaunch=false regardless of blue machine state")
}

// TestDeployWithStoppedBlueMachinesEnforcesHealthChecks verifies the full
// deploy pipeline when blue machines have SkipLaunch=true (auto-stopped).
//
// Before the fix, the deploy would silently succeed: green machines were
// never started and their health was faked as "1/1 passing".
//
// After the fix, the deploy must attempt real health checks and only succeed
// when they pass, or fail/roll back when they don't.
func TestDeployWithStoppedBlueMachinesEnforcesHealthChecks(t *testing.T) {
	t.Run("fails when health checks cannot be verified", func(t *testing.T) {
		// breakGet=true simulates the platform being unreachable for health polls.
		client := &mockFlapsClient{breakGet: true}
		ctx := context.Background()
		ctx = flapsutil.NewContextWithClient(ctx, client)

		strategy := newBlueGreenStrategyWithState(client, fly.MachineStateStopped, true)
		// Short timeout so the test doesn't hang.
		strategy.timeout = 500 * time.Millisecond

		err := strategy.Deploy(ctx)
		assert.Error(t, err,
			"deploy must fail when health checks cannot be verified, not silently succeed")
	})

	t.Run("succeeds when health checks pass", func(t *testing.T) {
		// Default mockFlapsClient.Get returns a passing machine.
		client := &mockFlapsClient{}
		ctx := context.Background()
		ctx = flapsutil.NewContextWithClient(ctx, client)

		strategy := newBlueGreenStrategyWithState(client, fly.MachineStateStopped, true)

		err := strategy.Deploy(ctx)
		assert.NoError(t, err, "deploy must succeed when health checks pass")
	})
}
