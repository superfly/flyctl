package machine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/mock"
)

func newLaunchTestClient(t *testing.T) *mock.FlapsClient {
	t.Helper()

	return &mock.FlapsClient{
		// Default List returns nothing \u2014 tests override as needed.
		ListFunc: func(ctx context.Context, appName, state string) ([]*fly.Machine, error) {
			return nil, nil
		},
	}
}

func transient408() error {
	return &flaps.FlapsError{
		OriginalError:      fmt.Errorf("upstream timeout"),
		ResponseStatusCode: http.StatusRequestTimeout,
		ResponseBody:       []byte("upstream timeout"),
	}
}

// TestLaunchWithIdempotency_HappyPath verifies the common case: Launch
// succeeds on the first attempt, no lookup needed.
func TestLaunchWithIdempotency_HappyPath(t *testing.T) {
	client := newLaunchTestClient(t)
	launchCalls := atomic.Int32{}
	client.LaunchFunc = func(ctx context.Context, appName string, in fly.LaunchMachineInput) (*fly.Machine, error) {
		launchCalls.Add(1)

		return &fly.Machine{ID: "m1", Config: in.Config}, nil
	}

	m, err := LaunchWithIdempotency(
		context.Background(), client, "test-app",
		&fly.LaunchMachineInput{Config: &fly.MachineConfig{}},
		LaunchIdempotencyOpts{RetryAttempts: 3},
	)
	require.NoError(t, err)
	assert.Equal(t, "m1", m.ID)
	assert.Equal(t, int32(1), launchCalls.Load(), "no retry needed on happy path")
	assert.NotEmpty(t, m.Config.Metadata[FlyctlLaunchIDMetadataKey], "launch-id must be stamped even on the first attempt")
}

// TestLaunchWithIdempotency_RetriesTransientFailures covers the case where
// several transient 408s happen back-to-back with no side effect on the
// server. The retry loop must keep trying until Launch actually lands.
func TestLaunchWithIdempotency_RetriesTransientFailures(t *testing.T) {
	client := newLaunchTestClient(t)
	launchCalls := atomic.Int32{}
	client.LaunchFunc = func(ctx context.Context, appName string, in fly.LaunchMachineInput) (*fly.Machine, error) {
		n := launchCalls.Add(1)
		if n <= 2 {
			return nil, transient408()
		}

		return &fly.Machine{ID: "m1", Config: in.Config}, nil
	}

	m, err := LaunchWithIdempotency(
		context.Background(), client, "test-app",
		&fly.LaunchMachineInput{Config: &fly.MachineConfig{}},
		LaunchIdempotencyOpts{RetryAttempts: 4},
	)
	require.NoError(t, err)
	assert.Equal(t, "m1", m.ID)
	assert.Equal(t, int32(3), launchCalls.Load())
}

// TestLaunchWithIdempotency_DetectsSilentSuccess covers the case where a
// Launch call committed the machine on the flaps side but returned a
// transient error. On retry the lookup finds the machine by its launch-id
// metadata and returns it without creating a duplicate.
func TestLaunchWithIdempotency_DetectsSilentSuccess(t *testing.T) {
	client := newLaunchTestClient(t)

	var committed []*fly.Machine
	launchCalls := atomic.Int32{}
	client.LaunchFunc = func(ctx context.Context, appName string, in fly.LaunchMachineInput) (*fly.Machine, error) {
		launchCalls.Add(1)
		// Commit on the mock side but return 408.
		m := &fly.Machine{
			ID:     fmt.Sprintf("m%d", len(committed)+1),
			Config: &fly.MachineConfig{Metadata: map[string]string{}},
		}
		for k, v := range in.Config.Metadata {
			m.Config.Metadata[k] = v
		}
		committed = append(committed, m)

		return nil, transient408()
	}
	client.ListFunc = func(ctx context.Context, appName, state string) ([]*fly.Machine, error) {
		return committed, nil
	}

	silentSuccessCalls := atomic.Int32{}
	m, err := LaunchWithIdempotency(
		context.Background(), client, "test-app",
		&fly.LaunchMachineInput{Config: &fly.MachineConfig{}},
		LaunchIdempotencyOpts{
			RetryAttempts: 3,
			OnSilentSuccess: func(_ *fly.Machine) {
				silentSuccessCalls.Add(1)
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "m1", m.ID, "must return the machine committed by the first (silent-success) attempt")
	assert.Equal(t, int32(1), launchCalls.Load(),
		"only one Launch call should be made \u2014 the lookup deduplicates before retry")
	assert.Len(t, committed, 1, "no duplicate machine should have been created")
	assert.Equal(t, int32(1), silentSuccessCalls.Load(), "OnSilentSuccess must be notified")
}

// TestLaunchWithIdempotency_NonTransientErrorFailsFast verifies that a
// validation error or other non-retryable failure surfaces immediately
// without wasting attempts.
func TestLaunchWithIdempotency_NonTransientErrorFailsFast(t *testing.T) {
	client := newLaunchTestClient(t)
	launchCalls := atomic.Int32{}
	client.LaunchFunc = func(ctx context.Context, appName string, in fly.LaunchMachineInput) (*fly.Machine, error) {
		launchCalls.Add(1)

		return nil, &flaps.FlapsError{
			OriginalError:      errors.New("bad request"),
			ResponseStatusCode: http.StatusBadRequest,
			ResponseBody:       []byte("bad request"),
		}
	}

	_, err := LaunchWithIdempotency(
		context.Background(), client, "test-app",
		&fly.LaunchMachineInput{Config: &fly.MachineConfig{}},
		LaunchIdempotencyOpts{RetryAttempts: 5},
	)
	assert.Error(t, err)
	assert.Equal(t, int32(1), launchCalls.Load(), "non-transient errors must fail on the first attempt")
}

// TestLaunchWithIdempotency_ExhaustsRetries verifies the helper eventually
// gives up and surfaces the last error.
func TestLaunchWithIdempotency_ExhaustsRetries(t *testing.T) {
	client := newLaunchTestClient(t)
	launchCalls := atomic.Int32{}
	client.LaunchFunc = func(ctx context.Context, appName string, in fly.LaunchMachineInput) (*fly.Machine, error) {
		launchCalls.Add(1)

		return nil, transient408()
	}

	_, err := LaunchWithIdempotency(
		context.Background(), client, "test-app",
		&fly.LaunchMachineInput{Config: &fly.MachineConfig{}},
		LaunchIdempotencyOpts{RetryAttempts: 3},
	)
	assert.Error(t, err, "exhausted retries must surface the last error")
	assert.Equal(t, int32(3), launchCalls.Load(), "should attempt exactly RetryAttempts times")
}

// TestLaunchWithIdempotency_UsesCallerProvidedLaunchID verifies the helper
// respects a launch-id the caller pre-set (useful when the caller wants
// the ID to be visible outside the helper for correlation).
func TestLaunchWithIdempotency_UsesCallerProvidedLaunchID(t *testing.T) {
	client := newLaunchTestClient(t)
	client.LaunchFunc = func(ctx context.Context, appName string, in fly.LaunchMachineInput) (*fly.Machine, error) {
		return &fly.Machine{ID: "m1", Config: in.Config}, nil
	}

	m, err := LaunchWithIdempotency(
		context.Background(), client, "test-app",
		&fly.LaunchMachineInput{Config: &fly.MachineConfig{
			Metadata: map[string]string{FlyctlLaunchIDMetadataKey: "caller-provided-id"},
		}},
		LaunchIdempotencyOpts{RetryAttempts: 2},
	)
	require.NoError(t, err)
	assert.Equal(t, "caller-provided-id", m.Config.Metadata[FlyctlLaunchIDMetadataKey])
}

// TestLaunchWithIdempotency_NilConfigReturnsError enforces the API contract
// that the caller must provide a Config \u2014 launching a machine without one
// is a bug we should catch loudly.
func TestLaunchWithIdempotency_NilConfigReturnsError(t *testing.T) {
	client := newLaunchTestClient(t)
	client.LaunchFunc = func(ctx context.Context, appName string, in fly.LaunchMachineInput) (*fly.Machine, error) {
		t.Fatal("Launch should not be called when Config is nil")

		return nil, nil
	}

	_, err := LaunchWithIdempotency(
		context.Background(), client, "test-app",
		&fly.LaunchMachineInput{Config: nil},
		LaunchIdempotencyOpts{RetryAttempts: 3},
	)
	assert.Error(t, err)
}

// TestLaunchWithIdempotency_ContextCancellation ensures cancellation between
// attempts short-circuits the retry loop.
func TestLaunchWithIdempotency_ContextCancellation(t *testing.T) {
	client := newLaunchTestClient(t)
	launchCalls := atomic.Int32{}
	ctx, cancel := context.WithCancel(context.Background())

	client.LaunchFunc = func(_ context.Context, _ string, _ fly.LaunchMachineInput) (*fly.Machine, error) {
		launchCalls.Add(1)
		cancel() // cancel between the first failure and the first retry.

		return nil, transient408()
	}

	_, err := LaunchWithIdempotency(
		ctx, client, "test-app",
		&fly.LaunchMachineInput{Config: &fly.MachineConfig{}},
		LaunchIdempotencyOpts{RetryAttempts: 5, LookupDelay: 10 * time.Millisecond},
	)
	assert.Error(t, err, "cancellation must surface as an error")
	assert.LessOrEqual(t, launchCalls.Load(), int32(2), "cancellation must stop retries quickly")
}
