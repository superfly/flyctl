package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

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
	vmCmd.Args = cobra.MaximumNArgs(1)
	vmCmd.AddIntFlag(IntFlagOpts{
		Name:        "memory",
		Description: "Memory in MB for the VM",
		Default:     0,
	})

	balanceCmdStrings := docstrings.Get("scale.balanced")
	balanceCmd := BuildCommand(cmd, runBalanceScale, balanceCmdStrings.Usage, balanceCmdStrings.Short, balanceCmdStrings.Long, os.Stdout, requireSession, requireAppName)
	balanceCmd.Args = cobra.RangeArgs(0, 2)

	standardCmdStrings := docstrings.Get("scale.standard")
	standardCmd := BuildCommand(cmd, runStandardScale, standardCmdStrings.Usage, standardCmdStrings.Short, standardCmdStrings.Long, os.Stdout, requireSession, requireAppName)
	standardCmd.Args = cobra.RangeArgs(0, 2)

	setCmdStrings := docstrings.Get("scale.set")
	setCmd := BuildCommand(cmd, runSetParamsOnly, setCmdStrings.Usage, setCmdStrings.Short, setCmdStrings.Long, os.Stdout, requireSession, requireAppName)
	setCmd.Args = cobra.RangeArgs(0, 2)

	showCmdStrings := docstrings.Get("scale.show")
	BuildCommand(cmd, runShow, showCmdStrings.Usage, showCmdStrings.Short, showCmdStrings.Long, os.Stdout, requireSession, requireAppName)

	return cmd
}

func runBalanceScale(commandContext *cmdctx.CmdContext) error {
	return actualScale(commandContext, true, false)
}

func runStandardScale(commandContext *cmdctx.CmdContext) error {
	return actualScale(commandContext, false, false)
}

func runSetParamsOnly(commandContext *cmdctx.CmdContext) error {
	return actualScale(commandContext, false, true)
}

func actualScale(commandContext *cmdctx.CmdContext, balanceRegions bool, setParamsOnly bool) error {
	currentcfg, err := commandContext.Client.API().AppAutoscalingConfig(commandContext.AppName)
	if err != nil {
		return err
	}

	newcfg := api.UpdateAutoscaleConfigInput{AppID: commandContext.AppName}

	newcfg.BalanceRegions = &balanceRegions
	newcfg.MinCount = &currentcfg.MinCount
	newcfg.MaxCount = &currentcfg.MaxCount

	kvargs := make(map[string]string)

	for _, pair := range commandContext.Args {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("Scale parameters must be provided as NAME=VALUE pairs (%s is invalid)", pair)
		}
		key := parts[0]
		value := parts[1]
		kvargs[strings.ToLower(key)] = value
	}

	minval, found := kvargs["min"]

	if found {
		minint64val, err := strconv.ParseInt(minval, 10, 64)

		if err != nil {
			return errors.New("could not parse min count value")
		}
		minintval := int(minint64val)
		newcfg.MinCount = &minintval
		delete(kvargs, "min")
	}

	maxval, found := kvargs["max"]

	if found {
		maxint64val, err := strconv.ParseInt(maxval, 10, 64)

		if err != nil {
			return errors.New("could not parse max count value")
		}
		maxintval := int(maxint64val)
		newcfg.MaxCount = &maxintval
		delete(kvargs, "max")
	}

	if len(kvargs) != 0 {
		unusedkeys := ""
		for k := range kvargs {
			if unusedkeys == "" {
				unusedkeys = k
			} else {
				unusedkeys = unusedkeys + ", " + k
			}
		}
		return errors.New("unrecognised parameters in command:" + unusedkeys)
	}

	cfg, err := commandContext.Client.API().UpdateAutoscaleConfig(newcfg)
	if err != nil {
		return err
	}

	printScaleConfig(commandContext, cfg)

	return nil
}

func runScaleVM(commandContext *cmdctx.CmdContext) error {
	if len(commandContext.Args) == 0 {
		size, err := commandContext.Client.API().AppVMSize(commandContext.AppName)
		if err != nil {
			return err
		}

		fmt.Printf("%15s: %s\n", "Size", size.Name)
		fmt.Printf("%15s: %s\n", "CPU Cores", formatCores(size))
		fmt.Printf("%15s: %s\n", "Memory", formatMemory(size))
		return nil
	}

	// kvargs := make(map[string]string)
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

func runShow(commandContext *cmdctx.CmdContext) error {
	cfg, err := commandContext.Client.API().AppAutoscalingConfig(commandContext.AppName)
	if err != nil {
		return err
	}
	size, err := commandContext.Client.API().AppVMSize(commandContext.AppName)
	if err != nil {
		return err
	}

	printScaleConfig(commandContext, cfg)

	printSize(commandContext, size)

	return nil
}

func printScaleConfig(commandContext *cmdctx.CmdContext, cfg *api.AutoscalingConfig) {

	asJSON := commandContext.OutputJSON()

	if asJSON {
		commandContext.WriteJSON(cfg)
	} else {
		var mode string

		if cfg.BalanceRegions {
			mode = "Balanced"
		} else {
			mode = "Standard"
		}

		fmt.Fprintf(commandContext.Out, "%15s: %s\n", "Scale Mode", mode)
		fmt.Fprintf(commandContext.Out, "%15s: %d\n", "Min Count", cfg.MinCount)
		fmt.Fprintf(commandContext.Out, "%15s: %d\n", "Max Count", cfg.MaxCount)
	}
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
