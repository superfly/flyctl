package volumes

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func TestRunDestroySafeguards(t *testing.T) {
	for _, tc := range []struct {
		name      string
		args      []string
		app, want string
	}{
		{"missing target", nil, "", "volume ID or app required"},
		{"headless requires yes", []string{"vol_one"}, "selected-app", "yes flag must be specified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newDestroy()
			require.NoError(t, cmd.ParseFlags(tc.args))
			ios, _, out, _ := iostreams.Test()
			ctx := iostreams.NewContext(flag.NewContext(t.Context(), cmd.Flags()), ios)
			ctx = appconfig.WithName(ctx, tc.app)
			deleted := 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetVolumeFunc: func(_ context.Context, app, id string) (*fly.Volume, error) {
					require.Equal(t, "selected-app", app)
					require.Equal(t, "vol_one", id)
					return &fly.Volume{ID: id, Name: "data"}, nil
				},
				GetVolumesFunc:   func(context.Context, string) ([]fly.Volume, error) { return []fly.Volume{{Name: "data"}}, nil },
				DeleteVolumeFunc: func(context.Context, string, string) (*fly.Volume, error) { deleted++; return nil, nil },
			})
			require.ErrorContains(t, runDestroy(ctx), tc.want)
			require.Zero(t, deleted)
			require.Empty(t, out.String())
		})
	}
}

func TestRunDestroyBatch(t *testing.T) {
	denied := errors.New("volume is attached")
	for _, tc := range []struct {
		name     string
		inferApp bool
		failAt   string
	}{
		{name: "explicit app"},
		{name: "app inferred from volume", inferApp: true},
		{name: "partial failure stops batch", failAt: "vol_two"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newDestroy()
			require.NoError(t, cmd.ParseFlags([]string{"--yes", "vol_one", "vol_two", "vol_three"}))
			ios, _, out, _ := iostreams.Test()
			ctx := iostreams.NewContext(flag.NewContext(t.Context(), cmd.Flags()), ios)
			if !tc.inferApp {
				ctx = appconfig.WithName(ctx, "selected-app")
			}
			inferred := 0
			ctx = flyutil.NewContextWithClient(ctx, &mock.Client{
				GetAppNameFromVolumeFunc: func(_ context.Context, id string) (*string, error) {
					inferred++
					require.Equal(t, "vol_one", id)
					return fly.Pointer("selected-app"), nil
				},
			})
			var deleted []string
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				DeleteVolumeFunc: func(_ context.Context, app, id string) (*fly.Volume, error) {
					require.Equal(t, "selected-app", app)
					deleted = append(deleted, id)
					if id == tc.failAt {
						return nil, denied
					}

					return &fly.Volume{ID: id, Name: "data"}, nil
				},
			})
			err := runDestroy(ctx)
			if tc.failAt == "" {
				require.NoError(t, err)
				require.Equal(t, []string{"vol_one", "vol_two", "vol_three"}, deleted)
				require.Contains(t, out.String(), "Destroyed volume ID: vol_three name: data")
			} else {
				require.ErrorIs(t, err, denied)
				require.Equal(t, []string{"vol_one", "vol_two"}, deleted)
				require.Equal(t, "Destroyed volume ID: vol_one name: data\n", out.String())
			}
			if tc.inferApp {
				require.Equal(t, 1, inferred)
			} else {
				require.Zero(t, inferred)
			}
		})
	}
}
