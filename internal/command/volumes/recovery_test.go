package volumes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func TestHeadlessVolumeDiscovery(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	volume, err := selectVolume(iostreams.NewContext(context.Background(), ios), nil, "test-app")
	require.Nil(t, volume)
	require.ErrorIs(t, err, prompt.ErrNonInteractive)
	require.ErrorContains(t, err, "fly volumes list --app <app-name>")
}

func TestMissingVolumeSizeNamesFlagAndUnits(t *testing.T) {
	for _, size := range []string{"", "0"} {
		t.Run("size="+size, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			cmd := newExtend()
			args := []string{"vol-1"}
			if size != "" {
				args = append(args, "--size="+size)
			}
			require.NoError(t, cmd.Flags().Parse(args))
			ctx := flag.NewContext(iostreams.NewContext(context.Background(), ios), cmd.Flags())
			ctx = config.NewContext(ctx, &config.Config{})
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetAppFunc: func(context.Context, string) (*flaps.App, error) { return &flaps.App{}, nil },
			})
			err := runExtend(ctx)
			require.ErrorContains(t, err, "--size in gigabytes")
			require.ErrorContains(t, err, "--size 10")
		})
	}
}
