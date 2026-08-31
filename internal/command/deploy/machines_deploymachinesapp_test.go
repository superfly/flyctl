package deploy

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyerr"
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

func TestVolumePlacementCapacitySuggestion(t *testing.T) {
	placementErr := &flaps.FlapsError{
		OriginalError: errors.New("failed_precondition: insufficient resources for volumes"),
		ResponseBody:  []byte(`{"status":"volume_placement_capacity"}`),
		FlyRequestId:  "request-id",
	}

	t.Run("adds deploy advice and preserves Flaps metadata", func(t *testing.T) {
		err := withVolumePlacementCapacitySuggestion(fmt.Errorf("failed to create machine: %w", placementErr))

		require.ErrorIs(t, err, placementErr)
		require.Equal(t, "request-id", flaps.GetErrorRequestID(err))
		require.Contains(t, flyerr.GetErrorSuggestion(err), "Try again later")
		require.Contains(t, flyerr.GetErrorSuggestion(err), "fly volume create")
		require.Contains(t, flyerr.GetErrorSuggestion(err), "empty volume")
	})

	t.Run("leaves unrelated errors unchanged", func(t *testing.T) {
		err := errors.New("machine launch failed")

		require.Same(t, err, withVolumePlacementCapacitySuggestion(err))
	})
}
