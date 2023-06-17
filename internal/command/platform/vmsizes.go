package platform

import (
	"context"
	"fmt"
	"sort"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newVMSizes() (cmd *cobra.Command) {
	const (
		long = `View a list of VM sizes which can be used with the FLYCTL SCALE VM command
`
		short = "List VM Sizes"
	)

	cmd = command.New("vm-sizes", short, long, runVMSizes,
		command.RequireSession,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd, flag.JSONOutput())
	return
}

func runVMSizes(ctx context.Context) error {
	client := client.FromContext(ctx).API()

	sizes, err := client.PlatformVMSizes(ctx)
	if err != nil {
		return fmt.Errorf("failed retrieving sizes: %w", err)
	}

	out := iostreams.FromContext(ctx).Out
	if config.FromContext(ctx).JSONOutput {
		return render.JSON(out, sizes)
	}

	var rows [][]string
	for _, size := range sizes {
		rows = append(rows, []string{
			size.Name,
			cores(int(size.CPUCores)),
			memory(size.MemoryMB),
		})
	}

	render.Table(out, "Nomad platform", rows, "Name", "CPU Cores", "Memory")

	return runMachineVMSizes(ctx)
}

func runMachineVMSizes(ctx context.Context) error {
	out := iostreams.FromContext(ctx).Out

	type preset struct {
		guest   *api.MachineGuest
		strings []string
	}
	sortedPresets := lo.MapToSlice(api.MachinePresets, func(key string, value *api.MachineGuest) preset {
		arr := []string{
			key,
			cores(value.CPUs),
			memory(value.MemoryMB),
		}
		return preset{value, arr}
	})
	sort.Slice(sortedPresets, func(i, j int) bool {
		a := sortedPresets[i].guest
		b := sortedPresets[j].guest
		if a.CPUs == b.CPUs {
			return a.MemoryMB < b.MemoryMB
		}
		return a.CPUs < b.CPUs
	})

	// Filter and display shared cpu sizes.
	shared := lo.FilterMap(sortedPresets, func(p preset, _ int) ([]string, bool) {
		if p.guest.CPUKind == "shared" {
			return p.strings, true
		}
		return nil, false
	})
	err := render.Table(out, "Machines platform", shared, "Name", "CPU Cores", "Memory")
	if err != nil {
		return fmt.Errorf("failed to render shared vm-sizes: %s", err)
	}

	// Filter and display performance cpu sizes.
	performance := lo.FilterMap(sortedPresets, func(p preset, _ int) ([]string, bool) {
		if p.guest.CPUKind == "performance" {
			return p.strings, true
		}
		return nil, false
	})
	return render.Table(out, "", performance, "Name", "CPU Cores", "Memory")
}

func cores(cores int) string {
	if cores < 1.0 {
		return fmt.Sprintf("%d", cores)
	}
	return fmt.Sprintf("%d", cores)
}

func memory(size int) string {
	if size < 1024 {
		return fmt.Sprintf("%d MB", size)
	}
	return fmt.Sprintf("%d GB", size/1024)
}
