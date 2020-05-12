package cmd

import (
	"errors"
	"fmt"
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

func runBalanceScale(ctx *CmdContext) error {
	return actualScale(ctx, true, false)
}

func runStandardScale(ctx *CmdContext) error {
	return actualScale(ctx, false, false)
}

func runSetParamsOnly(ctx *CmdContext) error {
	return actualScale(ctx, false, true)
}

func actualScale(ctx *CmdContext, balanceRegions bool, setParamsOnly bool) error {
	currentcfg, err := ctx.Client.API().AppAutoscalingConfig(ctx.AppName)

	newcfg := api.UpdateAutoscaleConfigInput{AppID: ctx.AppName}

	newcfg.BalanceRegions = &balanceRegions
	newcfg.MinCount = &currentcfg.MinCount
	newcfg.MaxCount = &currentcfg.MaxCount

	kvargs := make(map[string]string)

	for _, pair := range ctx.Args {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("Secrets must be provided as NAME=VALUE pairs (%s is invalid)", pair)
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

func runShow(ctx *CmdContext) error {
	cfg, err := ctx.Client.API().AppAutoscalingConfig(ctx.AppName)
	if err != nil {
		return err
	}
	size, err := ctx.Client.API().AppVMSize(ctx.AppName)
	if err != nil {
		return err
	}

	if cfg.BalanceRegions {
		fmt.Fprintln(ctx.Out, "Scale Mode: Balanced")
	} else {
		fmt.Fprintln(ctx.Out, "Scale Mode: Standard")
	}
	fmt.Fprintln(ctx.Out, "Min Count:", cfg.MinCount)
	fmt.Fprintln(ctx.Out, "Max Count:", cfg.MaxCount)
	fmt.Println("VM Size:", size.Name)

	return nil
}

func printScaleConfig(w io.Writer, cfg *api.AutoscalingConfig) {

	if cfg.BalanceRegions {
		fmt.Fprintln(w, "Scale Mode: Balanced")
	} else {
		fmt.Fprintln(w, "Scale Mode: Standard")
	}
	fmt.Fprintln(w, "Min Count:", cfg.MinCount)
	fmt.Fprintln(w, "Max Count:", cfg.MaxCount)
}
