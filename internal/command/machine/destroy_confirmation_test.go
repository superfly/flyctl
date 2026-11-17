//go:build !windows

package machine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func TestRunMachineDestroyImageSelection(t *testing.T) {
	for _, answer := range []string{"n", "y"} {
		t.Run(answer, func(t *testing.T) {
			cmd := newDestroy()
			require.NoError(t, cmd.ParseFlags([]string{"--image", "app:target", "--force"}))
			ios, out, _ := mock.ConfirmationIO(answer)
			ctx := iostreams.NewContext(flag.NewContext(t.Context(), cmd.Flags()), ios)
			ctx = appconfig.WithName(ctx, "selected-app")
			calls := 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				ListActiveFunc: func(_ context.Context, app string) ([]*fly.Machine, error) {
					require.Equal(t, "selected-app", app)
					return []*fly.Machine{
						{ID: "target", State: "started", ImageRef: fly.MachineImageRef{Registry: "registry.example", Repository: "app", Tag: "target"}},
						{ID: "unrelated", State: "started", ImageRef: fly.MachineImageRef{Registry: "registry.example", Repository: "app", Tag: "other"}},
					}, nil
				},
				GetAppFunc: func(_ context.Context, app string) (*flaps.App, error) { return &flaps.App{Name: app}, nil },
				DestroyFunc: func(_ context.Context, app string, input fly.RemoveMachineInput, _ string) error {
					calls++
					require.Equal(t, "selected-app", app)
					require.Equal(t, "target", input.ID)
					require.True(t, input.Kill)
					return nil
				},
			})
			require.NoError(t, runMachineDestroy(ctx))
			if answer == "y" {
				require.Equal(t, 1, calls)
			} else {
				require.Zero(t, calls)
			}
			require.NotContains(t, out.String(), "unrelated has been destroyed")
		})
	}
}
