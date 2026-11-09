package machine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func TestCloneCarriesOverMountAutoExtendSettings(t *testing.T) {
	const newVolumeID = "vol_clone"

	tests := []struct {
		name   string
		source fly.MachineMount
		want   fly.MachineMount
	}{
		{
			name: "percentage growth",
			source: fly.MachineMount{
				Volume:                 "vol_source",
				Name:                   "pgdata",
				Path:                   "/data",
				SizeGb:                 10,
				ExtendThresholdPercent: 70,
				AddSizePercent:         20,
				MinAddSizeGb:           5,
				SizeGbLimit:            1000,
			},
			want: fly.MachineMount{
				Volume:                 newVolumeID,
				Path:                   "/data",
				ExtendThresholdPercent: 70,
				AddSizePercent:         20,
				MinAddSizeGb:           5,
				SizeGbLimit:            1000,
			},
		},
		{
			name: "fixed growth",
			source: fly.MachineMount{
				Volume:                 "vol_source",
				Name:                   "data",
				Path:                   "/data",
				SizeGb:                 10,
				ExtendThresholdPercent: 80,
				AddSizeGb:              3,
				SizeGbLimit:            100,
			},
			want: fly.MachineMount{
				Volume:                 newVolumeID,
				Path:                   "/data",
				ExtendThresholdPercent: 80,
				AddSizeGb:              3,
				SizeGbLimit:            100,
			},
		},
		{
			name: "no auto-extend",
			source: fly.MachineMount{
				Volume: "vol_source",
				Name:   "data",
				Path:   "/data",
				SizeGb: 10,
			},
			want: fly.MachineMount{
				Volume: newVolumeID,
				Path:   "/data",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := &fly.Machine{
				ID:         "source",
				Region:     "lax",
				HostStatus: fly.HostStatusOk,
				Config: &fly.MachineConfig{
					Image:  "example/image:latest",
					Mounts: []fly.MachineMount{tt.source},
				},
			}

			var launched fly.LaunchMachineInput
			client := &mock.FlapsClient{
				GetFunc: func(context.Context, string, string) (*fly.Machine, error) {
					return source, nil
				},
				CreateVolumeFunc: func(context.Context, string, fly.CreateVolumeRequest) (*fly.Volume, error) {
					return &fly.Volume{ID: newVolumeID}, nil
				},
				LaunchFunc: func(_ context.Context, _ string, input fly.LaunchMachineInput) (*fly.Machine, error) {
					launched = input
					return &fly.Machine{ID: "clone"}, nil
				},
			}

			cmd := newClone()
			require.NoError(t, cmd.Flags().Parse([]string{source.ID, "--region", "fra", "--detach"}))

			ios, _, _, _ := iostreams.Test()
			ctx := iostreams.NewContext(context.Background(), ios)
			ctx = flag.NewContext(ctx, cmd.Flags())
			ctx = appconfig.WithName(ctx, "my-app")
			ctx = flapsutil.NewContextWithClient(ctx, client)

			require.NoError(t, runMachineClone(ctx))

			require.NotNil(t, launched.Config)
			assert.Equal(t, []fly.MachineMount{tt.want}, launched.Config.Mounts)
		})
	}
}
