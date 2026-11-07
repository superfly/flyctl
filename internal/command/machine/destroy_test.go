package machine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func TestDestroyStateAndForce(t *testing.T) {
	for _, tc := range []struct {
		state          string
		force, allowed bool
	}{
		{"stopped", false, true}, {"suspended", false, true},
		{"started", false, false}, {"started", true, true},
		{"created", false, false}, {"created", true, true},
		{"destroyed", false, false}, {"destroyed", true, false},
	} {
		t.Run(fmt.Sprintf("%s/force=%t", tc.state, tc.force), func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ctx := iostreams.NewContext(t.Context(), ios)
			calls := 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				DestroyFunc: func(_ context.Context, app string, input fly.RemoveMachineInput, nonce string) error {
					calls++
					require.Equal(t, "selected-app", app)
					require.Equal(t, "selected-machine", input.ID)
					require.Equal(t, tc.force, input.Kill)
					require.Equal(t, "held-lease", nonce)

					return nil
				},
			})
			err := Destroy(ctx, "selected-app", &fly.Machine{ID: "selected-machine", State: tc.state, LeaseNonce: "held-lease"}, tc.force)
			if tc.allowed {
				require.NoError(t, err)
				require.Equal(t, 1, calls)
			} else {
				require.Error(t, err)
				require.Zero(t, calls)
			}
		})
	}
}

func TestRunMachineDestroySelectionAndLeaseCleanup(t *testing.T) {
	deleteErr := errors.New("delete failed")
	for _, tc := range []struct {
		name                          string
		failDelete, cancel, failLease bool
	}{
		{name: "success"},
		{name: "deletion failure", failDelete: true},
		{name: "cancellation", cancel: true},
		{name: "partial lease acquisition failure", failLease: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newDestroy()
			require.NoError(t, cmd.ParseFlags([]string{"--force", "one", "two"}))
			ios, _, out, _ := iostreams.Test()
			if tc.cancel {
				// Cancelled-command cleanup reports lease releases concurrently.
				// Discard supports concurrent writes, like the real stdout file.
				ios.Out = io.Discard
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			ctx = iostreams.NewContext(flag.NewContext(ctx, cmd.Flags()), ios)
			ctx = appconfig.WithName(ctx, "selected-app")
			var mu sync.Mutex
			var acquired, released, deleted []string
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetFunc: func(_ context.Context, app, id string) (*fly.Machine, error) {
					require.Equal(t, "selected-app", app)
					require.Contains(t, []string{"one", "two"}, id)
					return &fly.Machine{ID: id, State: "started", HostStatus: fly.HostStatusOk, Version: "v1"}, nil
				},
				GetAppFunc: func(_ context.Context, app string) (*flaps.App, error) { return &flaps.App{Name: app}, nil },
				AcquireLeaseFunc: func(_ context.Context, app, id string, _ *int) (*fly.MachineLease, error) {
					mu.Lock()
					defer mu.Unlock()
					assert.Equal(t, "selected-app", app)
					if tc.failLease && id == "two" {
						return nil, errors.New("lease held by another process")
					}
					acquired = append(acquired, id)

					return &fly.MachineLease{Data: &fly.MachineLeaseData{Nonce: id + "-lease", Version: "v1"}}, nil
				},
				ReleaseLeaseFunc: func(ctx context.Context, app, id, nonce string) error {
					mu.Lock()
					defer mu.Unlock()
					assert.NoError(t, ctx.Err(), "cleanup must work after cancellation")
					assert.Equal(t, "selected-app", app)
					assert.Equal(t, id+"-lease", nonce)
					released = append(released, id)

					return nil
				},
				DestroyFunc: func(_ context.Context, app string, input fly.RemoveMachineInput, nonce string) error {
					require.Equal(t, "selected-app", app)
					require.True(t, input.Kill)
					require.Equal(t, input.ID+"-lease", nonce)
					deleted = append(deleted, input.ID)
					if tc.cancel {
						cancel()
						return context.Canceled
					}
					if tc.failDelete && len(deleted) == 2 {
						return deleteErr
					}

					return nil
				},
			})
			err := runMachineDestroy(ctx)
			switch {
			case tc.failLease:
				require.ErrorContains(t, err, "lease held by another process")
				require.Empty(t, deleted)
			case tc.cancel:
				require.ErrorIs(t, err, context.Canceled)
				require.Len(t, deleted, 1)
			case tc.failDelete:
				require.ErrorIs(t, err, deleteErr)
				require.Contains(t, out.String(), deleted[0]+" has been destroyed")
				require.NotContains(t, out.String(), deleted[1]+" has been destroyed")
			default:
				require.NoError(t, err)
				require.ElementsMatch(t, []string{"one", "two"}, deleted)
			}
			require.ElementsMatch(t, acquired, released, "release every acquired lease on every exit path")
		})
	}
}

func TestRunMachineDestroyHeadlessSelection(t *testing.T) {
	for _, args := range [][]string{nil, {"--image", "nginx:latest"}} {
		cmd := newDestroy()
		require.NoError(t, cmd.ParseFlags(args))
		ios, _, _, _ := iostreams.Test()
		ctx := iostreams.NewContext(flag.NewContext(t.Context(), cmd.Flags()), ios)
		ctx = appconfig.WithName(ctx, "selected-app")
		calls := 0
		ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
			ListActiveFunc: func(context.Context, string) ([]*fly.Machine, error) { return nil, nil },
			DestroyFunc:    func(context.Context, string, fly.RemoveMachineInput, string) error { calls++; return nil },
		})
		require.Error(t, runMachineDestroy(ctx))
		require.Zero(t, calls)
	}
}
