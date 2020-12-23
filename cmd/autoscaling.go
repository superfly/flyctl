package cmd

import (
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

func newAutoscaleCommand() *Command {
	autoscaleStrings := docstrings.Get("autoscale")

	cmd := BuildCommandKS(nil, nil, autoscaleStrings, os.Stdout, requireSession, requireAppName)
	cmd.Deprecated = "use `flyctl scale` instead"

	disableCmdStrings := docstrings.Get("autoscale.disable")
	disableCmd := BuildCommand(cmd, runDisableAutoscaling, disableCmdStrings.Usage, disableCmdStrings.Short, disableCmdStrings.Long, os.Stdout, requireSession, requireAppName)
	disableCmd.Args = cobra.RangeArgs(0, 2)

	balanceCmdStrings := docstrings.Get("autoscale.balanced")
	balanceCmd := BuildCommand(cmd, runBalanceScale, balanceCmdStrings.Usage, balanceCmdStrings.Short, balanceCmdStrings.Long, os.Stdout, requireSession, requireAppName)
	balanceCmd.Args = cobra.RangeArgs(0, 2)

	standardCmdStrings := docstrings.Get("autoscale.standard")
	standardCmd := BuildCommand(cmd, runStandardScale, standardCmdStrings.Usage, standardCmdStrings.Short, standardCmdStrings.Long, os.Stdout, requireSession, requireAppName)
	standardCmd.Args = cobra.RangeArgs(0, 2)

	setCmdStrings := docstrings.Get("autoscale.set")
	setCmd := BuildCommand(cmd, runSetParamsOnly, setCmdStrings.Usage, setCmdStrings.Short, setCmdStrings.Long, os.Stdout, requireSession, requireAppName)
	setCmd.Args = cobra.RangeArgs(0, 2)

	showCmdStrings := docstrings.Get("autoscale.show")
	BuildCommand(cmd, runAutoscalingShow, showCmdStrings.Usage, showCmdStrings.Short, showCmdStrings.Long, os.Stdout, requireSession, requireAppName)

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

func runDisableAutoscaling(commandContext *cmdctx.CmdContext) error {
	newcfg := api.UpdateAutoscaleConfigInput{AppID: commandContext.AppName, Enabled: api.BoolPointer(false)}

	cfg, err := commandContext.Client.API().UpdateAutoscaleConfig(newcfg)
	if err != nil {
		return err
	}

	printScaleConfig(commandContext, cfg)

	return nil
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

func runAutoscalingShow(commandContext *cmdctx.CmdContext) error {
	cfg, err := commandContext.Client.API().AppAutoscalingConfig(commandContext.AppName)
	if err != nil {
		return err
	}

	printScaleConfig(commandContext, cfg)

	return nil
}

func printScaleConfig(commandContext *cmdctx.CmdContext, cfg *api.AutoscalingConfig) {

	asJSON := commandContext.OutputJSON()

	if asJSON {
		commandContext.WriteJSON(cfg)
	} else {
		var mode string

		if !cfg.Enabled {
			mode = "Disabled"
		} else if cfg.BalanceRegions {
			mode = "Balanced"
		} else {
			mode = "Standard"
		}

		fmt.Fprintf(commandContext.Out, "%15s: %s\n", "Scale Mode", mode)
		if cfg.Enabled {
			fmt.Fprintf(commandContext.Out, "%15s: %d\n", "Min Count", cfg.MinCount)
			fmt.Fprintf(commandContext.Out, "%15s: %d\n", "Max Count", cfg.MaxCount)
		}
	}
}
