package deploy

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/superfly/fly-go"
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

	for i := 0; i < numberOfExistingMachines; i++ {
		machines = append(machines, &machineUpdateEntry{
			leasableMachine: machine.NewLeasableMachine(client, ios, &fly.Machine{}, false),
			launchInput: &fly.LaunchMachineInput{
				Config: &fly.MachineConfig{
					Metadata: map[string]string{},
					Checks: map[string]fly.MachineCheck{
						"check1": {},
					},
				},
			},
		})
	}
	strategy := &blueGreen{
		apiClient:     &mockWebClient{},
		flaps:         client,
		maxConcurrent: 10,
		appConfig:     &appconfig.Config{},
		io:            ios,
		colorize:      ios.ColorScheme(),
		timeout:       1 * time.Second,
		blueMachines:  machines,
	}
	strategy.initialize()

	// Don't have to wait during tests.
	strategy.waitBeforeStop = 0
	strategy.waitBeforeCordon = 0

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
