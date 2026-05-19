package machine

import (
	"context"
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

// Ensure the mock satisfies the interface at compile time.
var _ LeasableMachine = &leasableMachine{}

// Compile-time check: mock.FlapsClient must satisfy flapsutil.FlapsClient.
// (Imported indirectly; checked via the flaps package type.)
var _ interface {
	GetApp(context.Context, string) (*flaps.App, error)
} = &mock.FlapsClient{}
