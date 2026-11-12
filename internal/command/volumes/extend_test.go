package volumes

import (
	"context"
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

func TestExtendWithoutIDNonInteractive(t *testing.T) {
	for _, size := range []string{"10", "+10"} {
		t.Run(size, func(t *testing.T) {
			cmd := newExtend()
			require.NoError(t, cmd.Flags().Parse([]string{"--size", size}))
			ios, _, _, _ := iostreams.Test()
			ctx := iostreams.NewContext(context.Background(), ios)
			ctx = config.NewContext(ctx, &config.Config{})
			ctx = appconfig.WithName(ctx, "my-app")
			ctx = flag.NewContext(ctx, cmd.Flags())
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetAppFunc: func(context.Context, string) (*flaps.App, error) { return &flaps.App{}, nil },
				GetVolumeFunc: func(context.Context, string, string) (*fly.Volume, error) {
					t.Error("must select a volume before looking up its size")
					return &fly.Volume{}, nil
				},
			})
			require.EqualError(t, runExtend(ctx), "volume ID must be specified when not running interactively")
		})
	}
}
