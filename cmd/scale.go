package cmd

import (
	"errors"
	"fmt"
	"github.com/superfly/flyctl/cmdctx"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/superfly/flyctl/api"
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

func runBalanceScale(ctx *cmdctx.CmdContext) error {
	return actualScale(ctx, true, false)
}

func runStandardScale(ctx *cmdctx.CmdContext) error {
	return actualScale(ctx, false, false)
}

func runSetParamsOnly(ctx *cmdctx.CmdContext) error {
	return actualScale(ctx, false, true)
}

func actualScale(ctx *cmdctx.CmdContext, balanceRegions bool, setParamsOnly bool) error {
	currentcfg, err := ctx.Client.API().AppAutoscalingConfig(ctx.AppName)

	newcfg := api.UpdateAutoscaleConfigInput{AppID: ctx.AppName}

	newcfg.BalanceRegions = &balanceRegions
	newcfg.MinCount = &currentcfg.MinCount
	newcfg.MaxCount = &currentcfg.MaxCount

	kvargs := make(map[string]string)

	for _, pair := range ctx.Args {
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

	cfg, err := ctx.Client.API().UpdateAutoscaleConfig(newcfg)
	if err != nil {
		return err
	}

	printScaleConfig(ctx.Out, cfg)

	return nil
}

func runScaleVM(ctx *cmdctx.CmdContext) error {
	if len(ctx.Args) == 0 {
		size, err := ctx.Client.API().AppVMSize(ctx.AppName)
		if err != nil {
			return err
		}

		fmt.Printf("%15s: %s\n", "Size", size.Name)
		fmt.Printf("%15s: %s\n", "CPU Cores", formatCores(size))
		fmt.Printf("%15s: %s\n", "Memory", formatMemory(size))
		fmt.Printf("%15s: $%f\n", "Price (Month)", size.PriceMonth)
		fmt.Printf("%15s: $%f\n", "Price (Second)", size.PriceSecond)
		return nil
	}

	sizeName := ctx.Args[0]

	size, err := ctx.Client.API().SetAppVMSize(ctx.AppName, sizeName)
	if err != nil {
		return err
	}

	fmt.Println("Scaled VM size to", size.Name)
	fmt.Printf("%15s: %s\n", "CPU Cores", formatCores(size))
	fmt.Printf("%15s: %s\n", "Memory", formatMemory(size))
	fmt.Printf("%15s: $%f\n", "Price (Month)", size.PriceMonth)
	fmt.Printf("%15s: $%f\n", "Price (Second)", size.PriceSecond)
	return nil
}

func runShow(ctx *cmdctx.CmdContext) error {
	cfg, err := ctx.Client.API().AppAutoscalingConfig(ctx.AppName)
	if err != nil {
		return err
	}
	size, err := ctx.Client.API().AppVMSize(ctx.AppName)
	if err != nil {
		return err
	}

	printScaleConfig(ctx.Out, cfg)

	fmt.Fprintf(ctx.Out, "%15s: %s\n", "VM Size", size.Name)

	return nil
}

func printScaleConfig(w io.Writer, cfg *api.AutoscalingConfig) {

	mode := "Unknown"

	if cfg.BalanceRegions {
		mode = "Balanced"
	} else {
		mode = "Standard"
	}

	fmt.Fprintf(w, "%15s: %s\n", "Scale Mode", mode)
	fmt.Fprintf(w, "%15s: %d\n", "Min Count", cfg.MinCount)
	fmt.Fprintf(w, "%15s: %d\n", "Max Count", cfg.MaxCount)
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
