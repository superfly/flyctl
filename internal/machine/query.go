package machine

import (
	"context"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"golang.org/x/exp/slices"
)

func ListActive(ctx context.Context) ([]*api.Machine, error) {
	flapsClient := flaps.FromContext(ctx)

	machines, err := flapsClient.List(ctx, "")
	if err != nil {
		return nil, err
	}

	machines = lo.Filter(machines, func(m *api.Machine, _ int) bool {
		return m.Config != nil && m.IsActive() && !m.IsReleaseCommandMachine() && !m.IsFlyAppsConsole()
	})

	return machines, nil
}

// GetMedianGuest returns the median guest of a list of machines, or nil if no machines are provided.
// Calculated by sorting the machines by CPU count, taking the all machines with the median CPU count,
// and then sorting those by memory count and taking the median of those.
func GetMedianGuest(machines []*api.Machine) *api.MachineGuest {
	guests := lo.FilterMap(machines, func(m *api.Machine, _ int) (*api.MachineGuest, bool) {
		if m.Config == nil || m.Config.Guest == nil {
			return nil, false
		}
		return m.Config.Guest, true
	})
	if len(guests) == 0 {
		return nil
	}

	cpuCounts := lo.Map(guests, func(g *api.MachineGuest, _ int) int {
		return g.CPUs
	})
	slices.Sort(cpuCounts)
	medianCPUCount := cpuCounts[len(cpuCounts)/2]

	guests = lo.Filter(guests, func(g *api.MachineGuest, _ int) bool {
		return g.CPUs == medianCPUCount
	})
	slices.SortFunc(guests, func(a, b *api.MachineGuest) bool {
		return a.MemoryMB < b.MemoryMB
	})
	return guests[len(guests)/2]
}
