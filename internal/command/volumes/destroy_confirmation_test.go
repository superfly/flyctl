//go:build !windows

package volumes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func TestRunDestroyDeclined(t *testing.T) {
	cmd := newDestroy()
	require.NoError(t, cmd.ParseFlags([]string{"vol_one", "vol_two"}))
	ios, out, _ := mock.ConfirmationIO("n")
	ctx := iostreams.NewContext(flag.NewContext(t.Context(), cmd.Flags()), ios)
	ctx = appconfig.WithName(ctx, "selected-app")
	calls := 0
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetVolumeFunc: func(_ context.Context, app, id string) (*fly.Volume, error) {
			require.Equal(t, "selected-app", app)
			require.Equal(t, "vol_one", id)
			return &fly.Volume{ID: id, Name: "data"}, nil
		},
		GetVolumesFunc:   func(context.Context, string) ([]fly.Volume, error) { return []fly.Volume{{Name: "data"}}, nil },
		DeleteVolumeFunc: func(context.Context, string, string) (*fly.Volume, error) { calls++; return nil, nil },
	})
	require.NoError(t, runDestroy(ctx))
	require.Zero(t, calls)
	require.NotContains(t, out.String(), "Destroyed volume")
}
