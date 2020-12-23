package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/superfly/flyctl/cmdctx"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/docstrings"

	"github.com/spf13/cobra"
)

func newScaleCommand() *Command {
	scaleStrings := docstrings.Get("scale")

	cmd := BuildCommandKS(nil, nil, scaleStrings, os.Stdout, requireSession, requireAppName)

	vmCmdStrings := docstrings.Get("scale.vm")
	vmCmd := BuildCommand(cmd, runScaleVM, vmCmdStrings.Usage, vmCmdStrings.Short, vmCmdStrings.Long, os.Stdout, requireSession, requireAppName)
	vmCmd.Args = cobra.ExactArgs(1)
	vmCmd.AddIntFlag(IntFlagOpts{
		Name:        "memory",
		Description: "Memory in MB for the VM",
		Default:     0,
	})

	countCmdStrings := docstrings.Get("scale.count")
	countCmd := BuildCommand(cmd, runScaleCount, countCmdStrings.Usage, countCmdStrings.Short, countCmdStrings.Long, os.Stdout, requireSession, requireAppName)
	countCmd.Args = cobra.ExactArgs(1)

	showCmdStrings := docstrings.Get("scale.show")
	BuildCommand(cmd, runScaleShow, showCmdStrings.Usage, showCmdStrings.Short, showCmdStrings.Long, os.Stdout, requireSession, requireAppName)

	return cmd
}

func runScaleVM(commandContext *cmdctx.CmdContext) error {
	sizeName := commandContext.Args[0]

	memoryMB := int64(commandContext.Config.GetInt("memory"))

	size, err := commandContext.Client.API().SetAppVMSize(commandContext.AppName, sizeName, memoryMB)
	if err != nil {
		return err
	}

	fmt.Println("Scaled VM size to", size.Name)
	fmt.Printf("%15s: %s\n", "CPU Cores", formatCores(size))
	fmt.Printf("%15s: %s\n", "Memory", formatMemory(size))
	return nil
}

func runScaleCount(commandContext *cmdctx.CmdContext) error {
	count, err := strconv.Atoi(commandContext.Args[0])
	if err != nil {
		return err
	}

	counts, warnings, err := commandContext.Client.API().SetAppVMCount(commandContext.AppName, count)
	if err != nil {
		return err
	}

	if len(warnings) > 0 {
		for _, warning := range warnings {
			fmt.Println("Warning:", warning)
		}
		fmt.Println()
	}

	printTaskGroupCounts(commandContext, counts)

	return nil
}

func runScaleShow(commandContext *cmdctx.CmdContext) error {
	size, tgCounts, err := commandContext.Client.API().AppVMResources(commandContext.AppName)
	if err != nil {
		return err
	}
	fmt.Printf("VM Resources for %s\n", commandContext.AppName)
	printSize(commandContext, size)
	fmt.Println()
	printTaskGroupCounts(commandContext, tgCounts)

	return nil
}

func printSize(commandContext *cmdctx.CmdContext, cfg api.VMSize) {
	asJSON := commandContext.OutputJSON()

	if asJSON {
		prettyJSON, _ := json.MarshalIndent(cfg, "", "    ")
		fmt.Fprintln(commandContext.Out, string(prettyJSON))
	} else {
		fmt.Fprintf(commandContext.Out, "%15s: %s\n", "VM Size", cfg.Name)
		fmt.Fprintf(commandContext.Out, "%15s: %s\n", "VM Memory", formatMemory(cfg))
	}
}

func printTaskGroupCounts(commandContext *cmdctx.CmdContext, counts []api.TaskGroupCount) {
	if commandContext.OutputJSON() {
		prettyJSON, _ := json.MarshalIndent(counts, "", "    ")
		fmt.Fprintln(commandContext.Out, string(prettyJSON))
		return
	}

	fmt.Println("VM counts")
	for _, groupCount := range counts {
		fmt.Printf("%s: %d\n", groupCount.Name, groupCount.Count)
	}
}

// TODO: Move these funcs (also in presenters.VMSizes into presentation package)
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
