package image

import (
	"context"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func TestUpdateImageSkipsUnchangedMachines(t *testing.T) {
	const current = "registry.fly.io/test-app:current"

	machines := func(images ...string) (out []*fly.Machine) {
		for _, image := range images {
			out = append(out, &fly.Machine{
				ID:         "machine" + image[len(image)-1:],
				State:      "started",
				HostStatus: fly.HostStatusOk,
				Config:     &fly.MachineConfig{Image: image},
			})
		}

		return out
	}

	for _, tc := range []struct {
		name     string
		machines []*fly.Machine
		yes      bool
		wantErr  string
		wantOut  string
	}{
		{"all current", machines(current, current), false, "", "No changes to apply"},
		{"all current with yes", machines(current, current), true, "", "No changes to apply"},
		// Getting past the first Machine shows as the prompt failing, since
		// the test has no terminal.
		{"one outdated", machines(current, "registry.fly.io/test-app:old"), false, "yes flag must be specified", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &mock.FlapsClient{
				ListFunc: func(context.Context, string, string) ([]*fly.Machine, error) { return tc.machines, nil },
				AcquireLeaseFunc: func(context.Context, string, string, *int) (*fly.MachineLease, error) {
					return &fly.MachineLease{Data: &fly.MachineLeaseData{Nonce: "nonce"}}, nil
				},
				ReleaseLeaseFunc: func(context.Context, string, string, string) error { return nil },
				UpdateFunc: func(context.Context, string, fly.LaunchMachineInput, string) (*fly.Machine, error) {
					t.Fatal("unexpected call to Update on FlapsClient")
					return nil, nil
				},
			}
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flags.Bool("yes", tc.yes, "")
			flags.Bool("skip-health-checks", false, "")
			flags.String("image", current, "")
			io, _, out, _ := iostreams.Test()
			ctx := iostreams.NewContext(t.Context(), io)
			ctx = flag.NewContext(ctx, flags)
			ctx = flapsutil.NewContextWithClient(ctx, client)

			err := updateImageForMachines(ctx, &flaps.App{Name: "test-app"})
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}
			require.Contains(t, out.String(), tc.wantOut)
		})
	}
}
