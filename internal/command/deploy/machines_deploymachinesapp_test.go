package deploy

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func TestUpdateExistingMachinesWRecovery(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	client := &mockFlapsClient{}
	client.machines = []*fly.Machine{{ID: "test-machine-id", LeaseNonce: "foobar"}}
	md := &machineDeployment{
		app:         &flaps.App{},
		io:          ios,
		colorize:    ios.ColorScheme(),
		flapsClient: client,
		strategy:    "canary",
	}

	ctx := context.Background()
	err := md.updateExistingMachinesWRecovery(ctx, nil)
	assert.NoError(t, err)

	err = md.updateExistingMachinesWRecovery(ctx, []*machineUpdateEntry{
		{
			leasableMachine: machine.NewLeasableMachine(client, ios, "", &fly.Machine{}, false),
			launchInput:     &fly.LaunchMachineInput{},
		},
	})
	assert.Error(t, err, "failed to find machine test-machine-id")
}

func TestDeployMachinesApp(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	client := &mockFlapsClient{}
	webClient := &mock.Client{
		GetAppLogsFunc: func(ctx context.Context, appName, token, region, instanceID string) (entries []fly.LogEntry, nextToken string, err error) {
			return nil, "", nil
		},
	}
	client.machines = []*fly.Machine{
		{ID: "m1", LeaseNonce: "m1-lease", Config: &fly.MachineConfig{Metadata: map[string]string{fly.MachineConfigMetadataKeyFlyProcessGroup: "app"}}},
		{ID: "m2", LeaseNonce: "m2-lease", Config: &fly.MachineConfig{Metadata: map[string]string{fly.MachineConfigMetadataKeyFlyProcessGroup: "app"}}},
		{ID: "m3", LeaseNonce: "m3-lease", Config: &fly.MachineConfig{Metadata: map[string]string{fly.MachineConfigMetadataKeyFlyProcessGroup: "app"}}},
		{ID: "m4", LeaseNonce: "m4-lease", Config: &fly.MachineConfig{Metadata: map[string]string{fly.MachineConfigMetadataKeyFlyProcessGroup: "app"}}},
	}
	md := &machineDeployment{
		app:             &flaps.App{},
		io:              ios,
		colorize:        ios.ColorScheme(),
		flapsClient:     client,
		apiClient:       webClient,
		strategy:        "canary",
		appConfig:       &appconfig.Config{},
		machineSet:      machine.NewMachineSet(client, ios, "", client.machines, false),
		skipSmokeChecks: true,
		waitTimeout:     1 * time.Second,
	}

	// Shorten the NATS timeout since it's likely to require the fallback in CI
	natsConnectTimeout = md.waitTimeout

	ctx := context.Background()
	ctx = iostreams.NewContext(ctx, ios)
	ctx = flapsutil.NewContextWithClient(ctx, client)
	err := md.deployMachinesApp(ctx)
	assert.NoError(t, err)
}

// TestDeployCanaryKeepsInstanceDuringRoll guards against issue #4745: a canary
// deploy must not destroy the canary machine before the existing machines are
// rolled. Destroying the canary first leaves a window with no extra serving
// instance, which causes downtime for single-machine (n=1) apps. The canary
// must outlive the roll so that n+1 instances exist throughout it.
func TestDeployCanaryKeepsInstanceDuringRoll(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	client := &mockFlapsClient{}
	webClient := &mock.Client{
		GetAppLogsFunc: func(ctx context.Context, appName, token, region, instanceID string) (entries []fly.LogEntry, nextToken string, err error) {
			return nil, "", nil
		},
	}
	// The n=1 case from the issue: a single existing serving machine.
	client.machines = []*fly.Machine{
		{ID: "m1", LeaseNonce: "m1-lease", Config: &fly.MachineConfig{Metadata: map[string]string{fly.MachineConfigMetadataKeyFlyProcessGroup: "app"}}},
	}
	md := &machineDeployment{
		app:             &flaps.App{},
		io:              ios,
		colorize:        ios.ColorScheme(),
		flapsClient:     client,
		apiClient:       webClient,
		strategy:        "canary",
		appConfig:       &appconfig.Config{},
		machineSet:      machine.NewMachineSet(client, ios, "", client.machines, false),
		skipSmokeChecks: true,
		waitTimeout:     1 * time.Second,
	}

	// Shorten the NATS timeout since it's likely to require the fallback in CI
	natsConnectTimeout = md.waitTimeout

	ctx := context.Background()
	ctx = iostreams.NewContext(ctx, ios)
	ctx = flapsutil.NewContextWithClient(ctx, client)
	err := md.deployMachinesApp(ctx)
	assert.NoError(t, err)

	client.mu.Lock()
	defer client.mu.Unlock()

	require.NotEmpty(t, client.canaryIDs, "expected a canary machine to be launched")
	canaryID := client.canaryIDs[0]

	canaryDestroyIdx := client.eventIndex("destroy:" + canaryID)
	require.GreaterOrEqual(t, canaryDestroyIdx, 0,
		"canary machine %s was never destroyed; events=%v", canaryID, client.events)

	// The existing machine must be rolled (destroyed and replaced, or updated)
	// before the canary is torn down, so that there is always an extra serving
	// instance available during the roll.
	rollIdx := client.eventIndex("destroy:m1")
	require.GreaterOrEqual(t, rollIdx, 0,
		"expected existing machine m1 to be rolled; events=%v", client.events)

	assert.Greater(t, canaryDestroyIdx, rollIdx,
		"canary %s must be destroyed AFTER the existing machine is rolled to avoid downtime; events=%v",
		canaryID, client.events)
}
