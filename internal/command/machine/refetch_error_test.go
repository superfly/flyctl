package machine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagnames"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

var errRefetch = errors.New("refetch failed")

// Get returns a nil machine along with an error, so the error messages can't
// read the ID from its result.
func failingGet(context.Context, string, string) (*fly.Machine, error) {
	return nil, errRefetch
}

func TestStopReportsFailedRefetch(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Duration("wait-timeout", time.Minute, "")
	ctx := flag.NewContext(t.Context(), flags)
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		StopFunc: func(context.Context, string, fly.StopMachineInput, string) error { return nil },
		GetFunc:  failingGet,
	})

	err := Stop(ctx, "test-app", &fly.Machine{ID: "test-machine"}, "", 0)
	require.ErrorIs(t, err, errRefetch)
	require.ErrorContains(t, err, "could not get machine test-machine")
}

func TestSuspendReportsFailedRefetch(t *testing.T) {
	ctx := flapsutil.NewContextWithClient(t.Context(), &mock.FlapsClient{
		SuspendFunc: func(context.Context, string, string, string) error { return nil },
		GetFunc:     failingGet,
	})

	err := suspend(ctx, "test-app", &fly.Machine{ID: "test-machine"}, time.Minute)
	require.ErrorIs(t, err, errRefetch)
	require.ErrorContains(t, err, "could not get Machine test-machine")
}

func TestWaitReportsFailedRefetch(t *testing.T) {
	for _, tc := range []struct {
		name    string
		waitErr error
		want    string
	}{
		{"before retrying", errors.New("connection reset by peer"), "machine test-machine could not be refetched"},
		{"after settling", nil, "machine test-machine reached settled state"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gets := 0
			client := &mock.FlapsClient{
				GetFunc: func(ctx context.Context, app, id string) (*fly.Machine, error) {
					if gets++; gets == 1 {
						return &fly.Machine{ID: id}, nil
					}

					return failingGet(ctx, app, id)
				},
				WaitFunc: func(context.Context, string, string, ...flaps.WaitOption) error { return tc.waitErr },
			}

			cmd := newWait()
			require.NoError(t, cmd.Flags().Parse([]string{"test-machine"}))
			io, _, _, _ := iostreams.Test()
			ctx := iostreams.NewContext(t.Context(), io)
			ctx = flag.NewContext(ctx, cmd.Flags())
			ctx = appconfig.WithName(ctx, "test-app")
			ctx = flapsutil.NewContextWithClient(ctx, client)

			err := runMachineWait(ctx)
			require.ErrorIs(t, err, errRefetch)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestAPIProxyBindsToLocalhostByDefault(t *testing.T) {
	ctx := flag.NewContext(t.Context(), newProxy().Flags())
	require.Equal(t, "127.0.0.1", flag.GetBindAddr(ctx), "flag %s", flagnames.BindAddr)
}
