package platform

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/pkg/iostreams"
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
			cores(size),
			memory(size),
		})
	}

	return render.Table(out, "", rows, "Name", "CPU Cores", "Memory")
}

func cores(size api.VMSize) string {
	if size.CPUCores < 1.0 {
		return fmt.Sprintf("%.2f", size.CPUCores)
	}
	return fmt.Sprintf("%d", int(size.CPUCores))
}

func memory(size api.VMSize) string {
	if size.MemoryGB < 1.0 {
		return fmt.Sprintf("%d MB", size.MemoryMB)
	}
	return fmt.Sprintf("%d GB", int(size.MemoryGB))
}
