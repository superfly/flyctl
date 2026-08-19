package machine

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

// newTestLeasableMachine creates a leasableMachine wired to the provided
// flaps mock and seeded with the given fly.Machine value.
func newTestLeasableMachine(client *mock.FlapsClient, m *fly.Machine) *leasableMachine {
	ios, _, _, _ := iostreams.Test()

	return &leasableMachine{
		flapsClient: client,
		io:          ios,
		colorize:    ios.ColorScheme(),
		appName:     "test-app",
		machine:     m,
	}
}

// passingGetFunc returns a Get function that always responds with a machine
// whose named check is in the Passing state.
func passingGetFunc(checkName string) func(context.Context, string, string) (*fly.Machine, error) {
	return func(_ context.Context, _ string, machineID string) (*fly.Machine, error) {
		return &fly.Machine{
			ID: machineID,
			Checks: []*fly.MachineCheckStatus{
				{Name: checkName, Status: fly.Passing},
			},
		}, nil
	}
}

// failingGetFunc returns a Get function that always responds with a machine
// whose named check is in the Critical (failing) state.
func failingGetFunc(checkName string) func(context.Context, string, string) (*fly.Machine, error) {
	return func(_ context.Context, _ string, machineID string) (*fly.Machine, error) {
		return &fly.Machine{
			ID: machineID,
			Checks: []*fly.MachineCheckStatus{
				{Name: checkName, Status: fly.Critical},
			},
		}, nil
	}
}

// TestWaitForHealthchecksToPass_NilConfig verifies that a machine with a nil
// Config does not panic and returns immediately (no checks = nothing to wait for).
func TestWaitForHealthchecksToPass_NilConfig(t *testing.T) {
	client := &mock.FlapsClient{}
	lm := newTestLeasableMachine(client, &fly.Machine{ID: "m1", Config: nil})

	err := lm.WaitForHealthchecksToPass(context.Background(), 5*time.Second)
	assert.NoError(t, err)
}

// TestWaitForHealthchecksToPass_NoConfiguredChecks verifies that a machine
// with an empty Config (no checks defined) returns immediately without
// calling the API at all.
func TestWaitForHealthchecksToPass_NoConfiguredChecks(t *testing.T) {
	getCalls := atomic.Int32{}
	client := &mock.FlapsClient{
		GetFunc: func(ctx context.Context, appName, machineID string) (*fly.Machine, error) {
			getCalls.Add(1)

			return &fly.Machine{ID: machineID}, nil
		},
	}

	lm := newTestLeasableMachine(client, &fly.Machine{
		ID:     "m1",
		Config: &fly.MachineConfig{}, // no checks
	})

	err := lm.WaitForHealthchecksToPass(context.Background(), 5*time.Second)
	assert.NoError(t, err)
	assert.Equal(t, int32(0), getCalls.Load(), "Get should not be called when no checks are configured")
}

// TestWaitForHealthchecksToPass_TotalZeroDoesNotPass verifies that when the
// platform has not yet reported any check results (Total == 0), the function
// keeps waiting instead of exiting early.
//
// Prior to the fix, AllPassing() returned true vacuously when Total == 0
// (because 0 == 0), causing the function to exit immediately on the very
// first poll before any real check result arrived.
func TestWaitForHealthchecksToPass_TotalZeroDoesNotPass(t *testing.T) {
	calls := atomic.Int32{}
	client := &mock.FlapsClient{
		GetFunc: func(ctx context.Context, appName, machineID string) (*fly.Machine, error) {
			n := calls.Add(1)
			if n == 1 {
				// First poll: platform hasn't reported any results yet.
				return &fly.Machine{
					ID:     machineID,
					Checks: []*fly.MachineCheckStatus{}, // Total == 0
				}, nil
			}
			// Second poll: checks are now passing.
			return &fly.Machine{
				ID: machineID,
				Checks: []*fly.MachineCheckStatus{
					{Name: "alive", Status: fly.Passing},
				},
			}, nil
		},
	}

	lm := newTestLeasableMachine(client, &fly.Machine{
		ID: "m1",
		Config: &fly.MachineConfig{
			Checks: map[string]fly.MachineCheck{"alive": {}},
		},
	})

	err := lm.WaitForHealthchecksToPass(context.Background(), 10*time.Second)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, calls.Load(), int32(2),
		"should poll at least twice: once with no results, once with passing results")
}

// TestWaitForHealthchecksToPass_PassesWhenAllChecksPass verifies the happy
// path: configured checks, platform reports them as passing → returns nil.
func TestWaitForHealthchecksToPass_PassesWhenAllChecksPass(t *testing.T) {
	client := &mock.FlapsClient{GetFunc: passingGetFunc("alive")}

	lm := newTestLeasableMachine(client, &fly.Machine{
		ID: "m1",
		Config: &fly.MachineConfig{
			Checks: map[string]fly.MachineCheck{"alive": {}},
		},
	})

	err := lm.WaitForHealthchecksToPass(context.Background(), 5*time.Second)
	assert.NoError(t, err)
}

// TestWaitForHealthchecksToPass_TimesOutWhenChecksFail verifies that when
// checks are configured but consistently failing, the function eventually
// returns a timeout error rather than hanging forever.
func TestWaitForHealthchecksToPass_TimesOutWhenChecksFail(t *testing.T) {
	client := &mock.FlapsClient{GetFunc: failingGetFunc("alive")}

	lm := newTestLeasableMachine(client, &fly.Machine{
		ID: "m1",
		Config: &fly.MachineConfig{
			Checks: map[string]fly.MachineCheck{"alive": {}},
		},
	})

	err := lm.WaitForHealthchecksToPass(context.Background(), 300*time.Millisecond)
	assert.Error(t, err, "should return an error when checks never pass within the timeout")
}

// TestWaitForHealthchecksToPass_ServiceChecksAreIncluded verifies that
// service-level checks (Config.Services[*].Checks) are counted as configured
// checks, not just top-level Config.Checks.
func TestWaitForHealthchecksToPass_ServiceChecksAreIncluded(t *testing.T) {
	getCalls := atomic.Int32{}
	client := &mock.FlapsClient{
		GetFunc: func(ctx context.Context, appName, machineID string) (*fly.Machine, error) {
			getCalls.Add(1)

			return &fly.Machine{
				ID: machineID,
				Checks: []*fly.MachineCheckStatus{
					{Name: "servicecheck-00-http-8080", Status: fly.Passing},
				},
			}, nil
		},
	}

	lm := newTestLeasableMachine(client, &fly.Machine{
		ID: "m1",
		Config: &fly.MachineConfig{
			// No top-level checks; only a service-level check.
			Services: []fly.MachineService{
				{
					Checks: []fly.MachineServiceCheck{
						{Type: fly.StringPointer("http")},
					},
				},
			},
		},
	})

	err := lm.WaitForHealthchecksToPass(context.Background(), 5*time.Second)
	assert.NoError(t, err)
	assert.Greater(t, getCalls.Load(), int32(0), "Get should be called to poll service checks")
}

// TestWaitForHealthchecksToPass_ToleratesTransientGetErrors verifies the
// poll loop keeps going when flaps returns a transient error (408 / 5xx /
// network hiccup). Before the fix a single transient blip during health
// polling would abort the entire wait — and, therefore, the deploy.
func TestWaitForHealthchecksToPass_ToleratesTransientGetErrors(t *testing.T) {
	calls := atomic.Int32{}
	client := &mock.FlapsClient{
		GetFunc: func(ctx context.Context, appName, machineID string) (*fly.Machine, error) {
			n := calls.Add(1)
			if n <= 2 {
				return nil, &flaps.FlapsError{
					OriginalError:      fmt.Errorf("upstream timeout"),
					ResponseStatusCode: http.StatusRequestTimeout,
					ResponseBody:       []byte("upstream timeout"),
				}
			}

			return &fly.Machine{
				ID: machineID,
				Checks: []*fly.MachineCheckStatus{
					{Name: "alive", Status: fly.Passing},
				},
			}, nil
		},
	}

	lm := newTestLeasableMachine(client, &fly.Machine{
		ID: "m1",
		Config: &fly.MachineConfig{
			Checks: map[string]fly.MachineCheck{"alive": {}},
		},
	})

	err := lm.WaitForHealthchecksToPass(context.Background(), 10*time.Second)
	assert.NoError(t, err, "transient Get errors during health polling must not abort the wait")
	assert.GreaterOrEqual(t, calls.Load(), int32(3),
		"should poll past the two transient 408s and see the passing status")
}

// TestWaitForHealthchecksToPass_NonTransientErrorSurfaces verifies the wait
// loop still fails fast on genuinely permanent errors — we don't want to
// keep hammering flaps for something that will never succeed.
func TestWaitForHealthchecksToPass_NonTransientErrorSurfaces(t *testing.T) {
	calls := atomic.Int32{}
	client := &mock.FlapsClient{
		GetFunc: func(ctx context.Context, appName, machineID string) (*fly.Machine, error) {
			calls.Add(1)

			return nil, &flaps.FlapsError{
				OriginalError:      fmt.Errorf("forbidden"),
				ResponseStatusCode: http.StatusForbidden,
				ResponseBody:       []byte("forbidden"),
			}
		},
	}

	lm := newTestLeasableMachine(client, &fly.Machine{
		ID: "m1",
		Config: &fly.MachineConfig{
			Checks: map[string]fly.MachineCheck{"alive": {}},
		},
	})

	err := lm.WaitForHealthchecksToPass(context.Background(), 5*time.Second)
	assert.Error(t, err, "non-transient errors must be surfaced, not retried indefinitely")
	assert.Equal(t, int32(1), calls.Load(), "non-transient errors must fail on the first poll")
}

// TestWaitForEventType_ToleratesTransientGetErrors is the event-poll analogue
// of the health-check tolerance test.
func TestWaitForEventType_ToleratesTransientGetErrors(t *testing.T) {
	calls := atomic.Int32{}
	client := &mock.FlapsClient{
		GetFunc: func(ctx context.Context, appName, machineID string) (*fly.Machine, error) {
			n := calls.Add(1)
			if n <= 2 {
				return nil, &flaps.FlapsError{
					OriginalError:      fmt.Errorf("upstream timeout"),
					ResponseStatusCode: http.StatusRequestTimeout,
					ResponseBody:       []byte("upstream timeout"),
				}
			}

			return &fly.Machine{
				ID: machineID,
				Events: []*fly.MachineEvent{
					{Type: "exit"},
				},
			}, nil
		},
	}

	lm := newTestLeasableMachine(client, &fly.Machine{ID: "m1"})
	ev, err := lm.WaitForEventType(context.Background(), "exit", 10*time.Second, false)
	assert.NoError(t, err, "transient Get errors during event polling must not abort the wait")
	assert.NotNil(t, ev)
	assert.GreaterOrEqual(t, calls.Load(), int32(3))
}

// TestAcquireLease_RetriesTransientFailures verifies the lease-acquisition
// retry loop. AcquireLease is idempotent (a repeated request either
// returns the same lease or reports the current holder), so a single
// transient 408 or 5xx must not fail an otherwise-healthy deploy.
func TestAcquireLease_RetriesTransientFailures(t *testing.T) {
	calls := atomic.Int32{}
	client := &mock.FlapsClient{
		AcquireLeaseFunc: func(ctx context.Context, appName, machineID string, ttl *int) (*fly.MachineLease, error) {
			n := calls.Add(1)
			if n <= 2 {
				return nil, &flaps.FlapsError{
					OriginalError:      fmt.Errorf("upstream timeout"),
					ResponseStatusCode: http.StatusRequestTimeout,
					ResponseBody:       []byte("upstream timeout"),
				}
			}

			return &fly.MachineLease{
				Status: "success",
				Data:   &fly.MachineLeaseData{Nonce: "nonce-1"},
			}, nil
		},
	}
	lm := newTestLeasableMachine(client, &fly.Machine{ID: "m1"})

	err := lm.AcquireLease(context.Background(), 30*time.Second)
	assert.NoError(t, err, "transient 408s must be retried until AcquireLease lands")
	assert.Equal(t, int32(3), calls.Load(), "expected 2 failed + 1 successful attempt")
	assert.True(t, lm.HasLease())
}

// TestReleaseLease_RetriesTransientFailures verifies the release-side
// retry. ReleaseLease is idempotent (releasing an unknown nonce is a
// no-op on flaps), so retries are safe and preserve deploy hygiene.
func TestReleaseLease_RetriesTransientFailures(t *testing.T) {
	calls := atomic.Int32{}
	client := &mock.FlapsClient{
		ReleaseLeaseFunc: func(ctx context.Context, appName, machineID, nonce string) error {
			n := calls.Add(1)
			if n <= 2 {
				return &flaps.FlapsError{
					OriginalError:      fmt.Errorf("upstream timeout"),
					ResponseStatusCode: http.StatusRequestTimeout,
					ResponseBody:       []byte("upstream timeout"),
				}
			}

			return nil
		},
	}
	lm := newTestLeasableMachine(client, &fly.Machine{ID: "m1", LeaseNonce: "nonce-1"})
	lm.leaseNonce = "nonce-1"

	err := lm.ReleaseLease(context.Background())
	assert.NoError(t, err, "transient 408s must be retried until ReleaseLease lands")
	assert.Equal(t, int32(3), calls.Load(), "expected 2 failed + 1 successful attempt")
}

// Ensure the mock satisfies the interface at compile time.
var _ LeasableMachine = &leasableMachine{}

// Compile-time check: mock.FlapsClient must satisfy flapsutil.FlapsClient.
// (Imported indirectly; checked via the flaps package type.)
var _ interface {
	GetApp(context.Context, string) (*flaps.App, error)
} = &mock.FlapsClient{}
