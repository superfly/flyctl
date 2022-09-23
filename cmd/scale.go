package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/command"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/docstrings"

	"github.com/spf13/cobra"
)

func newScaleCommand(client *client.Client) *Command {
	scaleStrings := docstrings.Get("scale")

	cmd := BuildCommandKS(nil, nil, scaleStrings, client, requireSession, requireAppName)

	vmCmdStrings := docstrings.Get("scale.vm")
	vmCmd := BuildCommand(cmd, runScaleVM, vmCmdStrings.Usage, vmCmdStrings.Short, vmCmdStrings.Long, client, requireSession, requireAppName)
	vmCmd.Args = cobra.ExactArgs(1)
	vmCmd.AddIntFlag(IntFlagOpts{
		Name:        "memory",
		Description: "Memory in MB for the VM",
		Default:     0,
	})
	vmCmd.AddStringFlag(StringFlagOpts{
		Name:        "group",
		Description: "The process group to apply the VM size to",
		Default:     "",
	})

	memoryCmdStrings := docstrings.Get("scale.memory")
	memoryCmd := BuildCommandKS(cmd, runScaleMemory, memoryCmdStrings, client, requireSession, requireAppName)
	memoryCmd.Args = cobra.ExactArgs(1)
	memoryCmd.AddStringFlag(StringFlagOpts{
		Name:        "group",
		Description: "The process group to apply the memory size to",
		Default:     "",
	})

	countCmdStrings := docstrings.Get("scale.count")
	countCmd := BuildCommand(cmd, runScaleCount, countCmdStrings.Usage, countCmdStrings.Short, countCmdStrings.Long, client, requireSession, requireAppName)
	countCmd.Args = cobra.MinimumNArgs(1)
	countCmd.AddIntFlag((IntFlagOpts{
		Name:        "max-per-region",
		Description: "Max number of VMs per region",
		Default:     -1,
	}))

	showCmdStrings := docstrings.Get("scale.show")
	BuildCommand(cmd, runScaleShow, showCmdStrings.Usage, showCmdStrings.Short, showCmdStrings.Long, client, requireSession, requireAppName)

	return cmd
}

func runScaleVM(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()
	apiClient := cmdCtx.Client.API()

	isMachine, err := command.CheckPlatform(apiClient, ctx, cmdCtx.AppName)
	if err != nil {
		return fmt.Errorf("failed to check platform version %w", err)
	}

	if isMachine {
		return fmt.Errorf("it looks like your app is running on v2 of our platform, and does not support this legacy command: try running fly machine update instead")
	}

	sizeName := cmdCtx.Args[0]

	memoryMB := int64(cmdCtx.Config.GetInt("memory"))

	group := cmdCtx.Config.GetString("group")

	size, err := cmdCtx.Client.API().SetAppVMSize(ctx, cmdCtx.AppName, group, sizeName, memoryMB)
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

func runScaleCount(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()
	apiClient := cmdCtx.Client.API()

	isMachine, err := command.CheckPlatform(apiClient, ctx, cmdCtx.AppName)
	if err != nil {
		return fmt.Errorf("failed to check platform version %w", err)
	}

	if isMachine {
		return fmt.Errorf("it looks like your app is running on v2 of our platform, and does not support this legacy command: try running fly machine clone instead")
	}

	groups := map[string]int{}

	// single numeric arg: fly scale count 3
	if len(cmdCtx.Args) == 1 {
		count, err := strconv.Atoi(cmdCtx.Args[0])
		if err == nil {
			groups["app"] = count
		}
	}

	// group labels: fly scale web=X worker=Y
	if len(groups) < 1 {
		for _, arg := range cmdCtx.Args {
			parts := strings.Split(arg, "=")
			if len(parts) != 2 {
				return fmt.Errorf("%s is not a valid process=count option", arg)
			}
			count, err := strconv.Atoi(parts[1])
			if err != nil {
				return err
			}

			groups[parts[0]] = count
		}
	}

	// THIS IS AN OPTION TYPE CAN YOU TELL?
	maxPerRegionRaw := cmdCtx.Config.GetInt("max-per-region")
	maxPerRegion := &maxPerRegionRaw

	if maxPerRegionRaw == -1 {
		maxPerRegion = nil
	}

	counts, warnings, err := cmdCtx.Client.API().SetAppVMCount(ctx, cmdCtx.AppName, groups, maxPerRegion)
	if err != nil {
		return err
	}

	if len(warnings) > 0 {
		for _, warning := range warnings {
			fmt.Println("Warning:", warning)
		}
		fmt.Println()
	}

	msg := countMessage(counts)

	fmt.Printf("Count changed to %s\n", msg)

	return nil
}

func runScaleShow(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()
	apiClient := cmdCtx.Client.API()

	isMachine, err := command.CheckPlatform(apiClient, ctx, cmdCtx.AppName)
	if err != nil {
		return fmt.Errorf("failed to check platform version %w", err)
	}

	if isMachine {
		return fmt.Errorf("it looks like your app is running on v2 of our platform, and does not support this legacy command: try running fly machine status instead")
	}

	size, tgCounts, processGroups, err := cmdCtx.Client.API().AppVMResources(ctx, cmdCtx.AppName)
	if err != nil {
		return err
	}

	countMsg := countMessage(tgCounts)
	maxPerRegionMsg := maxPerRegionMessage(processGroups)

	printVMResources(cmdCtx, size, countMsg, maxPerRegionMsg)

	return nil
}

func countMessage(counts []api.TaskGroupCount) string {
	msg := ""

	if len(counts) == 1 {
		for _, tg := range counts {
			if tg.Name == "app" {
				msg = fmt.Sprint(tg.Count)
			}
		}
	}

	if msg == "" {
		for _, tg := range counts {
			msg += fmt.Sprintf("%s=%d ", tg.Name, tg.Count)
		}
	}

	return msg

	// return fmt.Sprintf("Count changed to %s\n", msg)
}

func maxPerRegionMessage(groups []api.ProcessGroup) string {
	msg := ""

	if len(groups) == 1 {
		for _, pg := range groups {
			if pg.Name == "app" {
				if pg.MaxPerRegion == 0 {
					msg = "Not set"
				} else {
					msg = fmt.Sprint(pg.MaxPerRegion)
				}
			}
		}
	}

	if msg == "" {
		for _, pg := range groups {
			msg += fmt.Sprintf("%s=%d ", pg.Name, pg.MaxPerRegion)
		}
	}

	return msg
}

func printVMResources(commandContext *cmdctx.CmdContext, vmSize api.VMSize, count string, maxPerRegion string) {
	if commandContext.OutputJSON() {
		out := struct {
			api.VMSize
			Count        string
			MaxPerRegion string
		}{
			VMSize:       vmSize,
			Count:        count,
			MaxPerRegion: maxPerRegion,
		}

		prettyJSON, _ := json.MarshalIndent(out, "", "    ")
		fmt.Fprintln(commandContext.Out, string(prettyJSON))
		return
	}

	fmt.Printf("VM Resources for %s\n", commandContext.AppName)

	fmt.Fprintf(commandContext.Out, "%15s: %s\n", "VM Size", vmSize.Name)
	fmt.Fprintf(commandContext.Out, "%15s: %s\n", "VM Memory", formatMemory(vmSize))
	fmt.Fprintf(commandContext.Out, "%15s: %s\n", "Count", count)
	fmt.Fprintf(commandContext.Out, "%15s: %s\n", "Max Per Region", maxPerRegion)
}

func runScaleMemory(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()
	apiClient := cmdCtx.Client.API()

	isMachine, err := command.CheckPlatform(apiClient, ctx, cmdCtx.AppName)
	if err != nil {
		return fmt.Errorf("failed to check platform version %w", err)
	}

	if isMachine {
		return fmt.Errorf("it looks like your app is running on v2 of our platform, and does not support this legacy command: try running fly machine update instead")
	}

	memoryMB, err := strconv.ParseInt(cmdCtx.Args[0], 10, 64)
	if err != nil {
		return err
	}

	// API doesn't allow memory setting on own yet, so get get the current size for the mutation
	currentsize, _, _, err := cmdCtx.Client.API().AppVMResources(ctx, cmdCtx.AppName)
	if err != nil {
		return err
	}

	group := cmdCtx.Config.GetString("group")

	size, err := cmdCtx.Client.API().SetAppVMSize(ctx, cmdCtx.AppName, group, currentsize.Name, memoryMB)
	if err != nil {
		return err
	}

	fmt.Println("Scaled VM Memory size to", formatMemory(size))
	fmt.Printf("%15s: %s\n", "CPU Cores", formatCores(size))
	fmt.Printf("%15s: %s\n", "Memory", formatMemory(size))

	return nil
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
