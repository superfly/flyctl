package scale

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
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
		})
	}

	fmt.Fprintf(io.Out, "VM Resources for app: %s\n\n", appName)
	render.Table(io.Out, "Groups", rows, "Name", "Count", "Kind", "CPUs", "Memory")

	return nil
}
