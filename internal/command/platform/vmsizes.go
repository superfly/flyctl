package platform

import (
	"context"
	"fmt"
	"sort"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newVMSizes() (cmd *cobra.Command) {
	const (
		long = `List the preset Machine sizes, with their CPU cores and memory, for shared
and performance CPUs. The list is built into flyctl. Use a size name with
` + "`fly scale vm`" + ` or the --vm-size flag. --json prints the sizes as JSON.`
		short = "List VM sizes"
	)

	cmd = command.New("vm-sizes", short, long, runMachineVMSizes,
		command.RequireSession,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd, flag.JSONOutput())

	return
}

func runMachineVMSizes(ctx context.Context) error {
	cfg := config.FromContext(ctx)
	out := iostreams.FromContext(ctx).Out

	type preset struct {
		guest   *fly.MachineGuest
		strings []string
	}

	// GPU presets are still in fly-go, but GPU machines are no longer offered.
	sortedPresets := lo.FilterMap(lo.Entries(fly.MachinePresets), func(e lo.Entry[string, *fly.MachineGuest], _ int) (preset, bool) {
		if e.Value.GPUKind != "" {
			return preset{}, false
		}

		arr := []string{
			e.Key,
			cores(e.Value.CPUs),
			memory(e.Value.MemoryMB),
		}

		return preset{e.Value, arr}, true
	})

	sort.Slice(sortedPresets, func(i, j int) bool {
		a := sortedPresets[i].guest
		b := sortedPresets[j].guest
		switch {
		case a.CPUs != b.CPUs:
			return a.CPUs < b.CPUs
		default:
			return a.MemoryMB < b.MemoryMB
		}
	})

	if cfg.JSONOutput {
		vmSizes := make(map[string]*fly.MachineGuest, len(sortedPresets))
		for _, preset := range sortedPresets {
			vmSizes[preset.strings[0]] = preset.guest
		}

		return render.JSON(out, vmSizes)
	}

	// Filter and display shared cpu sizes.
	shared := lo.FilterMap(sortedPresets, func(p preset, _ int) ([]string, bool) {
		return p.strings, p.guest.CPUKind == "shared"
	})
	if err := render.Table(out, "Machines platform", shared, "Name", "CPU Cores", "Memory"); err != nil {
		return err
	}

	// Filter and display performance cpu sizes.
	performance := lo.FilterMap(sortedPresets, func(p preset, _ int) ([]string, bool) {
		return p.strings, p.guest.CPUKind == "performance"
	})

	return render.Table(out, "", performance, "Name", "CPU Cores", "Memory")
}

func cores(cores int) string {
	return fmt.Sprintf("%d", cores)
}

func memory(size int) string {
	return fmt.Sprintf("%d MB", size)
}
