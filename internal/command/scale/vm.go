package scale

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newScaleVm() *cobra.Command {
	const (
		short = ""
		long  = ""
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
		fmt.Println("Scaled VM Type to\n", size.Name)
	} else {
		fmt.Printf("Scaled VM Type for \"%s\" to %s\n", group, size.Name)
	}
	fmt.Printf("%15s: %s\n", "CPU Cores", formatCores(size))
	fmt.Printf("%15s: %s\n", "Memory", formatMemory(size))
	return nil
}
