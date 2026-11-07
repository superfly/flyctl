package machine

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
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
