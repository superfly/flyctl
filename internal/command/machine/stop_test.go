package machine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
)

func TestStopRefetchFailure(t *testing.T) {
	const machineID = "machine-1"
	lookupErr := errors.New("machine no longer exists")
	client := &mock.FlapsClient{
		StopFunc: func(_ context.Context, app string, input fly.StopMachineInput, _ string) error {
			require.Equal(t, "test-app", app)
			require.Equal(t, machineID, input.ID)

			return nil
		},
		GetFunc: func(_ context.Context, app, id string) (*fly.Machine, error) {
			require.Equal(t, "test-app", app)
			require.Equal(t, machineID, id)

			return nil, lookupErr
		},
	}
	cmd := newStop()
	require.NoError(t, cmd.Flags().Parse([]string{"--wait-timeout=1s"}))
	ctx := flag.NewContext(context.Background(), cmd.Flags())
	ctx = flapsutil.NewContextWithClient(ctx, client)

	err := Stop(ctx, "test-app", &fly.Machine{ID: machineID}, "", 0)
	require.ErrorIs(t, err, lookupErr)
	require.EqualError(t, err, "could not get machine machine-1 to wait for stop: machine no longer exists")
}

func TestStopWaitCompletion(t *testing.T) {
	for _, waitTimeout := range []string{"0", "1s"} {
		t.Run(waitTimeout, func(t *testing.T) {
			getCalls, waitCalls := 0, 0
			client := &mock.FlapsClient{
				StopFunc: func(_ context.Context, app string, input fly.StopMachineInput, nonce string) error {
					require.Equal(t, "test-app", app)
					require.Equal(t, "machine-1", input.ID)
					require.Equal(t, "SIGTERM", input.Signal)
					require.Equal(t, 5*time.Second, input.Timeout.Duration)
					require.Equal(t, "lease", nonce)

					return nil
				},
				GetFunc: func(context.Context, string, string) (*fly.Machine, error) {
					getCalls++

					return &fly.Machine{ID: "machine-1"}, nil
				},
				WaitFunc: func(_ context.Context, app, id string, _ ...flaps.WaitOption) error {
					require.Equal(t, "test-app", app)
					require.Equal(t, "machine-1", id)
					waitCalls++

					return nil
				},
			}
			cmd := newStop()
			require.NoError(t, cmd.Flags().Parse([]string{"--wait-timeout=" + waitTimeout}))
			ctx := flapsutil.NewContextWithClient(flag.NewContext(context.Background(), cmd.Flags()), client)
			err := Stop(ctx, "test-app", &fly.Machine{ID: "machine-1", LeaseNonce: "lease"}, "sigterm", 5)
			require.NoError(t, err)
			wantCalls := 0
			if waitTimeout != "0" {
				wantCalls = 1
			}
			require.Equal(t, wantCalls, getCalls)
			require.Equal(t, wantCalls, waitCalls)
		})
	}
}
