package scale

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newScaleVm() *cobra.Command {
	const (
		short = "Change an app's VM to a named size (eg. shared-cpu-1x, performance-1x, performance-2x...)"
		long  = `Change an application's VM size to one of the named VM sizes.

For a full list of supported sizes use the command 'flyctl platform vm-sizes'

Memory size can be set with --memory=number-of-MB
e.g. flyctl scale vm shared-cpu-1x --memory=2048

For pricing, see https://fly.io/docs/about/pricing/`
	)
	cmd := command.New("vm [size]", short, long, runScaleVM,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.ExactArgs(1)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Int{
			Name:        "vm-memory",
			Description: "Memory in MB for the VM",
			Default:     0,
			Aliases:     []string{"memory"},
		},
		flag.ProcessGroup("The process group to apply the VM size to"),
	)
	return cmd
}

func runScaleVM(ctx context.Context) error {
	sizeName := flag.FirstArg(ctx)
	memoryMB := flag.GetInt(ctx, "vm-memory")
	group := flag.GetProcessGroup(ctx)
	return scaleVertically(ctx, group, sizeName, memoryMB)
}

func scaleVertically(ctx context.Context, group, sizeName string, memoryMB int) error {
	io := iostreams.FromContext(ctx)
	appName := appconfig.NameFromContext(ctx)

	size, err := v2ScaleVM(ctx, appName, group, sizeName, memoryMB)
	if err != nil {
		return err
	}

	if group == "" {
		fmt.Fprintf(io.Out, "Scaled VM Type to '%s'\n", size.Name)
	} else {
		fmt.Fprintf(io.Out, "Scaled VM Type for '%s' to '%s'\n", group, size.Name)
	}

	fmt.Fprintf(io.Out, "%15s: %s\n", "CPU Cores", formatCores(*size))
	fmt.Fprintf(io.Out, "%15s: %s\n", "Memory", formatMemory(*size))
	return nil
}

func formatCores(size fly.VMSize) string {
	if size.CPUCores < 1.0 {
		return fmt.Sprintf("%.2f", size.CPUCores)
	}
	return fmt.Sprintf("%d", int(size.CPUCores))
}

func formatMemory(size fly.VMSize) string {
	if size.MemoryGB < 1.0 {
		return fmt.Sprintf("%d MB", size.MemoryMB)
	}
	return fmt.Sprintf("%d GB", int(size.MemoryGB))
}
