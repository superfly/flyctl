package scale

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func runMachinesScaleShow(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	appName := appconfig.NameFromContext(ctx)

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return err
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	machines, _, err := flapsClient.ListFlyAppsMachines(ctx)
	if err != nil {
		return err
	}

	machines = lo.Filter(machines, func(m *fly.Machine, _ int) bool {
		return m.Config != nil
	})

	machineGroups := lo.GroupBy(machines, func(m *fly.Machine) string {
		return m.ProcessGroup()
	})

	// Deterministic output sorted by group name
	groupNames := lo.Keys(machineGroups)
	slices.Sort(groupNames)

	// TODO: Each machine can technically have a different Guest configuration.
	// It's impractical to show the guest for each machine, but arbitrarily
	// picking the first one is not ideal either.
	representativeGuests := lo.MapValues(machineGroups, func(machines []*fly.Machine, _ string) *fly.MachineGuest {
		if len(machines) == 0 {
			return nil
		}
		return machines[0].Config.Guest
	})

	if flag.GetBool(ctx, "json") {
		type groupData struct {
			Process string
			Count   int
			CPUKind string
			CPUs    int
			Memory  int
			Regions map[string]int
		}
		groups := lo.FilterMap(groupNames, func(name string, _ int) (res groupData, ok bool) {
			machines := machineGroups[name]
			guest := representativeGuests[name]
			if guest == nil {
				return res, false
			}
			return groupData{
				Process: name,
				Count:   len(machines),
				CPUKind: guest.CPUKind,
				CPUs:    guest.CPUs,
				Memory:  guest.MemoryMB,
				Regions: lo.CountValues(lo.Map(machines, func(m *fly.Machine, _ int) string {
					return m.Region
				})),
			}, true
		})

		prettyJSON, _ := json.MarshalIndent(groups, "", "    ")
		fmt.Fprintln(io.Out, string(prettyJSON))
		return nil
	}

	rows := make([][]string, 0, len(machineGroups))
	for _, groupName := range groupNames {
		machines := machineGroups[groupName]
		guest := representativeGuests[groupName]
		if guest == nil {
			continue
		}
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

func formatRegions(machines []*fly.Machine) string {
	regions := lo.Map(
		lo.Entries(lo.CountValues(lo.Map(machines, func(m *fly.Machine, _ int) string {
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
