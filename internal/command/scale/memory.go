package scale

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newScaleMemory() *cobra.Command {
	const (
		short = "Set VM memory"
		long  = `Set VM memory to a number of megabytes`
	)
	cmd := command.New("memory", short, long, runScaleMemory,
		command.RequireSession,
		command.RequireAppName,
		failOnMachinesApp,
	)
	cmd.Args = cobra.ExactArgs(1)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{Name: "group", Description: "The process group to apply the VM size to", Default: ""},
	)
	cmd.AddCommand()
	return cmd
}

func runScaleMemory(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	apiClient := client.FromContext(ctx).API()
	appName := appconfig.NameFromContext(ctx)
	group := flag.GetString(ctx, "group")

	memoryMB, err := strconv.ParseInt(flag.FirstArg(ctx), 10, 64)
	if err != nil {
		return err
	}

	// API doesn't allow memory setting on own yet, so get get the current size for the mutation
	currentsize, _, _, err := apiClient.AppVMResources(ctx, appName)
	if err != nil {
		return err
	}

	size, err := apiClient.SetAppVMSize(ctx, appName, group, currentsize.Name, memoryMB)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Scaled VM Memory size to %s\n", formatMemory(size))
	fmt.Fprintf(io.Out, "%15s: %s\n", "CPU Cores", formatCores(size))
	fmt.Fprintf(io.Out, "%15s: %s\n", "Memory", formatMemory(size))

	return nil
}

func formatCores(size api.VMSize) string {
	if size.CPUCores < 1.0 {
		return fmt.Sprintf("%.2f", size.CPUCores)
	}
	return fmt.Sprintf("%d", int(size.CPUCores))
}

func formatMemory(size api.VMSize) string {
	if size.MemoryGB < 1.0 {
		return fmt.Sprintf("%d MB", size.MemoryMB)
	}
	return fmt.Sprintf("%d GB", int(size.MemoryGB))
}
