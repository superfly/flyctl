package apps

import (
	"context"
	"errors"
	"slices"
	"strings"
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
			// Machines start on v1; an accepted update moves them to v2.
			machine := func(id, version string) *fly.Machine {
				return &fly.Machine{ID: id, Version: version, State: fly.MachineStateStopped, Config: &fly.MachineConfig{}}
			}
			var updated []string
			ios, _, _, _ := iostreams.Test()
			ctx := flapsutil.NewContextWithClient(iostreams.NewContext(t.Context(), ios), &mock.FlapsClient{
				UpdateFunc: func(_ context.Context, _ string, in fly.LaunchMachineInput, _ string) (*fly.Machine, error) {
					updated = append(updated, in.ID)
					if slices.Contains(tc.failed, in.ID) {
						return nil, denied
					}

					return machine(in.ID, "v2"), nil
				},
				WaitFunc: func(context.Context, string, string, ...flaps.WaitOption) error { return nil },
				GetFunc:  func(_ context.Context, _, id string) (*fly.Machine, error) { return machine(id, "v2"), nil },
			})

			err := updateMovedMachines(ctx, "app", []*fly.Machine{machine("m1", "v1"), machine("m2", "v1")}, true, nil)
			require.Equal(t, []string{"m1", "m2"}, updated)
			if tc.want == "" {
				require.NoError(t, err)

				return
			}
			require.ErrorIs(t, err, denied)
			require.ErrorContains(t, err, tc.want)
			for _, id := range tc.failed {
				// Named once: mach.Update's error already carries the ID.
				require.Equal(t, 1, strings.Count(err.Error(), id), err.Error())
			}
		})
	}
}

func TestMoveStopsUpdatingMachinesWhenCancelled(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ctx, cancel := context.WithCancel(iostreams.NewContext(t.Context(), ios))
	var updated []string
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		UpdateFunc: func(_ context.Context, _ string, in fly.LaunchMachineInput, _ string) (*fly.Machine, error) {
			updated = append(updated, in.ID)
			cancel() // The user presses Ctrl-C during the first update.

			return nil, context.Canceled
		},
	})

	machines := []*fly.Machine{
		{ID: "m1", Config: &fly.MachineConfig{}},
		{ID: "m2", Config: &fly.MachineConfig{}},
		{ID: "m3", Config: &fly.MachineConfig{}},
	}
	err := updateMovedMachines(ctx, "app", machines, true, nil)
	require.Equal(t, []string{"m1"}, updated)
	require.ErrorContains(t, err, "failed to update 1 of 3 Machines")
	require.ErrorContains(t, err, "stopped before updating 2 Machines")
}
