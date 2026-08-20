package deploy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/statuslogger"
	"github.com/superfly/flyctl/iostreams"
)

func TestWaitForMachineUsesCanaryTargetState(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	waitErr := errors.New("started wait should not run")
	waitCalls := 0
	client := &mock.FlapsClient{
		WaitFunc: func(context.Context, string, string, ...flaps.WaitOption) error {
			waitCalls++
			if waitCalls == 1 {
				return nil
			}

			return waitErr
		},
	}
	machineWithTarget := &fly.Machine{
		ID:          "machine-id",
		InstanceID:  "01G6R2TQGS41MBQTCA55X8ZCZW",
		TargetState: fly.MachineStateStopped,
	}
	entry := &machineUpdateEntry{
		leasableMachine: machine.NewLeasableMachine(client, ios, "app", machineWithTarget, false),
		launchInput:     &fly.LaunchMachineInput{},
	}
	md := &machineDeployment{
		io:          ios,
		strategy:    "canary",
		waitTimeout: time.Second,
	}
	ctx := iostreams.NewContext(context.Background(), ios)
	line := statuslogger.Create(ctx, 1, false).Line(0)

	err := md.waitForMachine(ctx, entry, line)
	assert.NoError(t, err)
	assert.Equal(t, 1, waitCalls, "canary must wait for the returned version to reach flyd's stopped target")

	md.strategy = "rolling"
	err = md.waitForMachine(ctx, entry, line)
	assert.Error(t, err)
	assert.Greater(t, waitCalls, 1)
	rollingWaitCalls := waitCalls

	md.strategy = "canary"
	machineWithTarget.TargetState = ""
	err = md.waitForMachine(ctx, entry, line)
	assert.Error(t, err, "an older server without target_state must retain the started wait")
	assert.Greater(t, waitCalls, rollingWaitCalls)
}

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
