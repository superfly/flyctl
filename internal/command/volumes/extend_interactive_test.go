//go:build !windows

package volumes

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

// A relative size needs the volume's current size, so the volume has to be
// picked before it's looked up.
func TestExtendByRelativeSizePicksVolumeFirst(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"picked from the list", []string{"--size", "+10"}},
		{"given as an argument", []string{"--size", "+10", "vol_one"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newExtend()
			require.NoError(t, cmd.ParseFlags(tc.args))
			ios, _, _ := mock.PromptIO()
			ios.In = selectInput{strings.NewReader("\r")}
			ctx := iostreams.NewContext(flag.NewContext(t.Context(), cmd.Flags()), ios)
			ctx = appconfig.WithName(config.NewContext(ctx, &config.Config{}), "app")
			volume := fly.Volume{ID: "vol_one", Name: "data", SizeGb: 5}
			extended := 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetAppFunc:     func(context.Context, string) (*flaps.App, error) { return &flaps.App{Name: "app"}, nil },
				GetVolumesFunc: func(context.Context, string) ([]fly.Volume, error) { return []fly.Volume{volume}, nil },
				GetVolumeFunc: func(_ context.Context, _, id string) (*fly.Volume, error) {
					require.Equal(t, "vol_one", id)

					return &volume, nil
				},
				ExtendVolumeFunc: func(_ context.Context, _, id string, size int) (*fly.Volume, bool, error) {
					extended++
					require.Equal(t, "vol_one", id)
					require.Equal(t, 15, size)

					return &volume, false, nil
				},
			})
			require.NoError(t, runExtend(ctx))
			require.Equal(t, 1, extended)
		})
	}
}

// selectInput feeds a Survey select raw keystrokes. Unlike a confirmation, it
// doesn't ask for the cursor position first, as mock.PromptIO assumes.
type selectInput struct{ *strings.Reader }

func (selectInput) Fd() uintptr  { return ^uintptr(0) }
func (selectInput) Close() error { return nil }
