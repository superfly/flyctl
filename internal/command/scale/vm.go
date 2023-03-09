package scale

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newScaleVm() *cobra.Command {
	const (
		short = "Change an app's VM to a named size (eg. shared-cpu-1x, dedicated-cpu-1x, dedicated-cpu-2x...)"
		long  = `Change an application's VM size to one of the named VM sizes.

For a full list of supported sizes use the command 'flyctl platform vm-sizes'

Memory size can be set with --memory=number-of-MB
e.g. flyctl scale vm shared-cpu-1x --memory=2048

For dedicated vms, this should be a multiple of 1024MB.
For shared vms, this can be 256MB or a a multiple of 1024MB.
For pricing, see https://fly.io/docs/about/pricing/`
	)
	cmd := command.New("vm", short, long, runScaleVM,
		command.RequireSession,
		command.RequireAppName,
		failOnMachinesApp,
	)
	cmd.Args = cobra.ExactArgs(1)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Int{Name: "memory", Description: "Memory in MB for the VM", Default: 0},
		flag.String{Name: "group", Description: "The process group to apply the VM size to", Default: ""},
	)
	cmd.AddCommand()
	return cmd
}

func runScaleVM(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	apiClient := client.FromContext(ctx).API()
	appName := appconfig.NameFromContext(ctx)
	sizeName := flag.FirstArg(ctx)
	group := flag.GetString(ctx, "group")
	memoryMB := int64(flag.GetInt(ctx, "memory"))

	size, err := apiClient.SetAppVMSize(ctx, appName, group, sizeName, memoryMB)
	if err != nil {
		return err
	}

	if group == "" {
		fmt.Fprintf(io.Out, "Scaled VM Type to '%s'\n", size.Name)
	} else {
		fmt.Fprintf(io.Out, "Scaled VM Type for '%s' to '%s'\n", group, size.Name)
	}
	fmt.Fprintf(io.Out, "%15s: %s\n", "CPU Cores", formatCores(size))
	fmt.Fprintf(io.Out, "%15s: %s\n", "Memory", formatMemory(size))
	return nil
}
