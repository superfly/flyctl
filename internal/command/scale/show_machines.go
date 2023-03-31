package scale

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
	"golang.org/x/exp/slices"
)

func runMachinesScaleShow(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	appName := appconfig.NameFromContext(ctx)

	flapsClient, err := flaps.NewFromAppName(ctx, appName)
	if err != nil {
		return err
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	machines, err := mach.ListActive(ctx)
	if err != nil {
		return err
	}

	machineGroups := lo.GroupBy(
		lo.Filter(machines, func(m *api.Machine, _ int) bool {
			return m.IsFlyAppsPlatform()
		}),
		func(m *api.Machine) string {
			return m.ProcessGroup()
		},
	)

	rows := make([][]string, 0, len(machineGroups))
	for groupName, machines := range machineGroups {
		guest := machines[0].Config.Guest

		rows = append(rows, []string{
			groupName,
			fmt.Sprintf("%d", len(machines)),
			guest.CPUKind,
			fmt.Sprintf("%d", guest.CPUs),
			fmt.Sprintf("%d MB", guest.MemoryMB),
			formatRegions(machines),
		})
	}

	fmt.Fprintf(io.Out, "VM Resources for app: %s\n\n", appName)
	render.Table(io.Out, "Groups", rows, "Name", "Count", "Kind", "CPUs", "Memory", "Regions")

	return nil
}

func formatRegions(machines []*api.Machine) string {
	regions := lo.Map(
		lo.Entries(lo.CountValues(lo.Map(machines, func(m *api.Machine, _ int) string {
			return m.Region
		}))),
		func(e lo.Entry[string, int], _ int) string {
			if e.Value > 1 {
				return fmt.Sprintf("%s(%d)", e.Key, e.Value)
			}
			return e.Key
		},
	)
	slices.Sort(regions)
	return strings.Join(regions, ",")
}
