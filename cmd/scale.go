package cmd

import (
	"fmt"
	"os"

	"github.com/superfly/flyctl/docstrings"

	"github.com/spf13/cobra"
)

func newScaleCommand() *Command {
	scaleStrings := docstrings.Get("scale")

	cmd := &Command{
		Command: &cobra.Command{
			Use:   scaleStrings.Usage,
			Short: scaleStrings.Short,
			Long:  scaleStrings.Long,
		},
	}

	vmCmdStrings := docstrings.Get("scale.vm")
	vmCmd := BuildCommand(cmd, runScaleVM, vmCmdStrings.Usage, vmCmdStrings.Short, vmCmdStrings.Long, os.Stdout, requireSession, requireAppName)
	vmCmd.Args = cobra.RangeArgs(0, 1)

	return cmd
}

func runScaleVM(ctx *CmdContext) error {
	if len(ctx.Args) == 0 {
		size, err := ctx.Client.API().AppVMSize(ctx.AppName)
		if err != nil {
			return err
		}

		fmt.Println("Size:", size.Name)
		fmt.Println("CPU Cores:", size.CPUCores)
		fmt.Println("Memory (GB):", size.MemoryGB)
		fmt.Println("Memory (MB):", size.MemoryMB)
		fmt.Println("Price (Month):", size.PriceMonth)
		fmt.Println("Price (Second):", size.PriceSecond)
		return nil
	}

	sizeName := ctx.Args[0]

	size, err := ctx.Client.API().SetAppVMSize(ctx.AppName, sizeName)
	if err != nil {
		return err
	}

	fmt.Println("Scaled VM size to", size.Name)

	fmt.Println("Size:", size.Name)
	fmt.Println("CPU Cores:", size.CPUCores)
	fmt.Println("Memory (GB):", size.MemoryGB)
	fmt.Println("Memory (MB):", size.MemoryMB)
	fmt.Println("Price (Month):", size.PriceMonth)
	fmt.Println("Price (Second):", size.PriceSecond)
	return nil
}
