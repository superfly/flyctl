package apps

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func TestMoveReportsMachineUpdateFailures(t *testing.T) {
	denied := errors.New("update denied")
	for _, tc := range []struct {
		name   string
		failed []string
		want   string
	}{
		{"all updated", nil, ""},
		{"one fails, the rest are still updated", []string{"m1"}, "failed to update 1 of 2 Machines"},
		{"all fail", []string{"m1", "m2"}, "failed to update 2 of 2 Machines"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			machine := func(id string) *fly.Machine {
				return &fly.Machine{ID: id, State: fly.MachineStateStopped, Config: &fly.MachineConfig{}}
			}
			var updated []string
			ios, _, _, _ := iostreams.Test()
			ctx := flapsutil.NewContextWithClient(iostreams.NewContext(t.Context(), ios), &mock.FlapsClient{
				UpdateFunc: func(_ context.Context, _ string, in fly.LaunchMachineInput, _ string) (*fly.Machine, error) {
					updated = append(updated, in.ID)
					if slices.Contains(tc.failed, in.ID) {
						return nil, denied
					}

					return machine(in.ID), nil
				},
				WaitFunc: func(context.Context, string, string, ...flaps.WaitOption) error { return nil },
				GetFunc:  func(_ context.Context, _, id string) (*fly.Machine, error) { return machine(id), nil },
			})

			err := updateMovedMachines(ctx, "app", []*fly.Machine{machine("m1"), machine("m2")}, true, nil)
			require.Equal(t, []string{"m1", "m2"}, updated)
			if tc.want == "" {
				require.NoError(t, err)

				return
			}
			require.ErrorIs(t, err, denied)
			require.ErrorContains(t, err, tc.want)
			for _, id := range tc.failed {
				require.ErrorContains(t, err, "machine "+id)
			}
		})
	}
}
