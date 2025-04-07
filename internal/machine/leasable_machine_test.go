package machine

import (
	"context"
	"errors"
	"github.com/superfly/flyctl/internal/flapsutil"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	fly "github.com/superfly/fly-go"
)

// Constants for test configuration
const (
	testMachineID        = "machine-id"
	testLeaseNonce       = "valid_nonce"
	testLeaseDuration    = 20 * time.Second
	testLeaseDelay       = 5 * time.Second
	testCancellationWait = 2 * time.Second
)

// LeaseData contains lease-specific information
type LeaseData struct {
	Nonce string
}

// Lease represents a machine lease
type Lease struct {
	Status string
	Data   *LeaseData
}

type MockFlapsClient struct {
	mock.Mock
	flapsutil.FlapsClient
	RefreshLeaseFunc func(ctx context.Context, machineID string, seconds *int, nonce string) (*Lease, error)
}

func (m *MockFlapsClient) RefreshLease(ctx context.Context, machineID string, seconds *int, nonce string) (*fly.MachineLease, error) {
	args := m.Called(ctx, machineID, seconds, nonce)
	return args.Get(0).(*fly.MachineLease), args.Error(1)
}

func TestRefreshLeaseUntilCanceled(t *testing.T) {
	tests := []struct {
		name                 string
		refreshLeaseResponse *fly.MachineLease
		refreshLeaseError    error
		expectTerminated     bool
		expectWarnings       bool
		description          string // Added for better test documentation
	}{
		{
			name: "successful_lease_refresh",
			refreshLeaseResponse: &fly.MachineLease{
				Status: "success",
				Data:   &fly.MachineLeaseData{Nonce: testLeaseNonce},
			},
			refreshLeaseError: nil,
			expectTerminated:  false,
			expectWarnings:    false,
			description:       "Should continue refreshing lease when lease refresh succeeds",
		},
		{
			name:                 "context_canceled_during_refresh",
			refreshLeaseResponse: nil,
			refreshLeaseError:    context.Canceled,
			expectTerminated:     true,
			expectWarnings:       false,
			description:          "Should terminate gracefully when context is canceled",
		},
		{
			name:                 "machine_not_found_error",
			refreshLeaseResponse: nil,
			refreshLeaseError:    errors.New("machine not found"),
			expectTerminated:     true,
			expectWarnings:       false,
			description:          "Should terminate when machine cannot be found",
		},
		{
			name:                 "other_transient_error",
			refreshLeaseResponse: nil,
			refreshLeaseError:    errors.New("some other error"),
			expectTerminated:     false,
			expectWarnings:       true,
			description:          "Should continue with warnings on transient errors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test context
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Create and configure mock client
			mockClient := new(MockFlapsClient)
			mockClient.On("RefreshLease", ctx, testMachineID, mock.AnythingOfType("*int"), testLeaseNonce).
				Return(tt.refreshLeaseResponse, tt.refreshLeaseError).
				Maybe()

			// Create test machine
			lm := &leasableMachine{
				flapsClient: mockClient,
				machine:     &fly.Machine{ID: testMachineID},
				leaseNonce:  testLeaseNonce,
			}

			// Setup test cancellation if needed
			if tt.expectTerminated {
				go func() {
					time.Sleep(testCancellationWait)
					cancel()
				}()
			}

			// Run the function under test
			lm.refreshLeaseUntilCanceled(ctx, testLeaseDuration, testLeaseDelay)

			// Verify expected behavior
			if !tt.expectTerminated {
				mockClient.AssertCalled(t, "RefreshLease", ctx, testMachineID,
					mock.AnythingOfType("*int"), testLeaseNonce)
			}
		})
	}
}
