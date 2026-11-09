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

// TestUpdateMachineInPlaceRetriesTransientFailures guards the retry loop
// wrapped around leasableMachine.Update. flaps.Update is nonce-scoped and
// therefore idempotent under retry, so a transient 408 (upstream
// context.DeadlineExceeded) that used to abort the whole rolling deploy
// mid-stream must now be transparently retried until the update lands.
func TestUpdateMachineInPlaceRetriesTransientFailures(t *testing.T) {
	t.Run("succeeds after transient 408s", func(t *testing.T) {
		ios, _, _, _ := iostreams.Test()
		client := &mockFlapsClient{updateTransientFailures: 2}
		md := &machineDeployment{
			app:         &flaps.App{Name: "test-app"},
			io:          ios,
			colorize:    ios.ColorScheme(),
			flapsClient: client,
		}

		// A lease must be held to call Update. Mirror what the deploy pipeline
		// does: pre-populate LeaseNonce so leasableMachine reports HasLease.
		lm := machine.NewLeasableMachine(client, ios, "test-app", &fly.Machine{ID: "m1", LeaseNonce: "m1-nonce"}, false)
		entry := &machineUpdateEntry{
			leasableMachine: lm,
			launchInput:     &fly.LaunchMachineInput{ID: "m1", Config: &fly.MachineConfig{}},
		}

		ctx := context.Background()
		err := md.updateMachineInPlace(ctx, entry)
		assert.NoError(t, err, "transient 408s must be retried until Update lands")

		client.mu.Lock()
		calls := client.updateCalls
		remaining := client.updateTransientFailures
		client.mu.Unlock()

		assert.Equal(t, 0, remaining, "all transient failures should be consumed by retries")
		assert.Equal(t, 3, calls, "expected 2 failed attempts + 1 successful attempt")
	})

	t.Run("gives up on non-transient errors after one attempt", func(t *testing.T) {
		ios, _, _, _ := iostreams.Test()
		// updateTransientFailures counter isn't set — the *first* call will
		// return the mock's baseline error. We want to observe that the
		// retry loop doesn't hammer flaps on non-408 errors.
		client := &mockFlapsClient{}
		// Force a permanent non-transient error via a wrapper.
		client.updateTransientFailures = 0
		md := &machineDeployment{
			app:         &flaps.App{Name: "test-app"},
			io:          ios,
			colorize:    ios.ColorScheme(),
			flapsClient: &nonTransientUpdateClient{mockFlapsClient: client},
		}

		lm := machine.NewLeasableMachine(md.flapsClient, ios, "test-app", &fly.Machine{ID: "m1", LeaseNonce: "m1-nonce"}, false)
		entry := &machineUpdateEntry{
			leasableMachine: lm,
			launchInput:     &fly.LaunchMachineInput{ID: "m1", Config: &fly.MachineConfig{}},
		}

		err := md.updateMachineInPlace(context.Background(), entry)
		assert.Error(t, err, "non-transient errors must surface")

		client.mu.Lock()
		calls := client.updateCalls
		client.mu.Unlock()
		assert.Equal(t, 1, calls, "non-transient errors must fail on the first attempt — no wasted retries")
	})
}

// nonTransientUpdateClient overrides Update to return a 400 (definitely
// non-transient) so we can test the retry classifier's negative path
// without touching every field on mockFlapsClient.
type nonTransientUpdateClient struct {
	*mockFlapsClient
}

func (c *nonTransientUpdateClient) Update(ctx context.Context, appName string, builder fly.LaunchMachineInput, nonce string) (*fly.Machine, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.updateCalls++

	return nil, &flaps.FlapsError{
		OriginalError:      assertErr("bad request"),
		ResponseStatusCode: 400,
		ResponseBody:       []byte("bad request"),
	}
}

// assertErr is a tiny helper so we don't need to import errors just for a
// literal error in a test.
type assertErr string

func (e assertErr) Error() string { return string(e) }

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
