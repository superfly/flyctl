package machine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func TestMachineWaitRefetch(t *testing.T) {
	for _, tc := range []struct {
		name       string
		state      string
		waitErr    error
		lookupErr  error
		wantErr    string
		wantOutput string
	}{
		{
			name:      "retry refetch fails",
			state:     "stopped",
			waitErr:   errors.New("connection reset by peer"),
			lookupErr: errors.New("machine no longer exists"),
			wantErr:   "machine machine-1 could not be refetched before retrying wait: machine no longer exists",
		},
		{
			name:      "final refetch fails",
			state:     "settled",
			lookupErr: errors.New("machine no longer exists"),
			wantErr:   "machine machine-1 reached settled state but could not fetch final state: machine no longer exists",
		},
		{
			name:       "final refetch succeeds",
			state:      "settled",
			wantOutput: "Machine machine-1 reached state \"stopped\"",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			getCalls, waitCalls := 0, 0
			client := &mock.FlapsClient{
				GetFunc: func(_ context.Context, app, id string) (*fly.Machine, error) {
					require.Equal(t, "test-app", app)
					require.Equal(t, "machine-1", id)
					getCalls++
					if getCalls > 1 && tc.lookupErr != nil {
						return nil, tc.lookupErr
					}

					return &fly.Machine{ID: id, State: "stopped"}, nil
				},
				WaitFunc: func(_ context.Context, app, id string, _ ...flaps.WaitOption) error {
					require.Equal(t, "test-app", app)
					require.Equal(t, "machine-1", id)
					waitCalls++

					return tc.waitErr
				},
			}
			cmd := newWait()
			require.NoError(t, cmd.Flags().Parse([]string{"--state=" + tc.state, "machine-1"}))
			ios, _, out, _ := iostreams.Test()
			ctx := iostreams.NewContext(context.Background(), ios)
			ctx = flag.NewContext(ctx, cmd.Flags())
			ctx = appconfig.WithName(ctx, "test-app")
			ctx = flapsutil.NewContextWithClient(ctx, client)

			err := runMachineWait(ctx)
			if tc.lookupErr != nil {
				require.ErrorIs(t, err, tc.lookupErr)
				require.EqualError(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
				require.Contains(t, strings.TrimSpace(out.String()), tc.wantOutput)
			}
			require.Equal(t, 2, getCalls)
			require.Equal(t, 1, waitCalls)
		})
	}
}

func TestMachineWaitCompletionAndDeadlines(t *testing.T) {
	terminalErr := errors.New("permission denied")
	for _, tc := range []struct {
		name    string
		waitErr error
		getSlow bool
		cancel  bool
		wantErr string
	}{
		{name: "already stopped"},
		{name: "terminal wait error", waitErr: terminalErr, wantErr: "permission denied"},
		{name: "deadline before next attempt", waitErr: errors.New("connection reset by peer"), wantErr: "did not reach \"stopped\" within 1s"},
		{name: "deadline during refetch", waitErr: errors.New("connection reset by peer"), getSlow: true, wantErr: "did not reach \"stopped\" within 1s"},
		{name: "cancel during retry", waitErr: errors.New("connection reset by peer"), cancel: true, wantErr: "context canceled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			getCalls := 0
			client := &mock.FlapsClient{
				GetFunc: func(context.Context, string, string) (*fly.Machine, error) {
					getCalls++
					if getCalls > 1 {
						if tc.getSlow {
							time.Sleep(time.Second + 10*time.Millisecond)
						}
						if tc.cancel {
							cancel()
						}
					}

					return &fly.Machine{ID: "machine-1"}, nil
				},
				WaitFunc: func(context.Context, string, string, ...flaps.WaitOption) error { return tc.waitErr },
			}
			cmd := newWait()
			require.NoError(t, cmd.Flags().Parse([]string{"--state=stopped", "--wait-timeout=1s", "machine-1"}))
			ios, _, out, _ := iostreams.Test()
			ctx = iostreams.NewContext(ctx, ios)
			ctx = flag.NewContext(ctx, cmd.Flags())
			ctx = appconfig.WithName(ctx, "test-app")
			ctx = flapsutil.NewContextWithClient(ctx, client)

			err := runMachineWait(ctx)
			if tc.wantErr == "" {
				require.NoError(t, err)
				require.Contains(t, out.String(), "Machine machine-1 reached state \"stopped\"")
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}
			if tc.waitErr == terminalErr {
				require.ErrorIs(t, err, terminalErr)
				require.Equal(t, 1, getCalls)
			}
			if tc.cancel {
				require.ErrorIs(t, err, context.Canceled)
			}
		})
	}
}
