package platform

import (
	"context"
	"fmt"
	"sort"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command"
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

	cmd = command.New("vm-sizes", short, long, runMachineVMSizes,
		command.RequireSession,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd, flag.JSONOutput())
	return
}

func runMachineVMSizes(ctx context.Context) error {
	out := iostreams.FromContext(ctx).Out

	type preset struct {
		guest   *fly.MachineGuest
		strings []string
	}

	sortedPresets := lo.MapToSlice(fly.MachinePresets, func(key string, value *fly.MachineGuest) preset {
		arr := []string{
			key,
			cores(value.CPUs),
			memory(value.MemoryMB),
			value.GPUKind,
		}
		return preset{value, arr}
	})

	sort.Slice(sortedPresets, func(i, j int) bool {
		a := sortedPresets[i].guest
		b := sortedPresets[j].guest
		switch {
		case a.CPUs != b.CPUs:
			return a.CPUs < b.CPUs
		case a.MemoryMB != b.MemoryMB:
			return a.MemoryMB < b.MemoryMB
		default:
			return a.GPUKind < b.GPUKind
		}
	})

	// Filter and display shared cpu sizes.
	shared := lo.FilterMap(sortedPresets, func(p preset, _ int) ([]string, bool) {
		return p.strings, p.guest.CPUKind == "shared" && p.guest.GPUKind == ""
	})
	if err := render.Table(out, "Machines platform", shared, "Name", "CPU Cores", "Memory"); err != nil {
		return err
	}

	// Filter and display performance cpu sizes.
	performance := lo.FilterMap(sortedPresets, func(p preset, _ int) ([]string, bool) {
		return p.strings, p.guest.CPUKind == "performance" && p.guest.GPUKind == ""
	})
	if err := render.Table(out, "", performance, "Name", "CPU Cores", "Memory"); err != nil {
		return err
	}

	// Filter and display gpu sizes.
	gpus := lo.FilterMap(sortedPresets, func(p preset, _ int) ([]string, bool) {
		return p.strings, p.guest.GPUKind != ""
	})
	return render.Table(out, "", gpus, "Name", "CPU Cores", "Memory", "GPU model")
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
