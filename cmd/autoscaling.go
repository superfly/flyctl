package cmd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/cmdctx"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/docstrings"

	"github.com/spf13/cobra"
)

func newAutoscaleCommand(client *client.Client) *Command {
	autoscaleStrings := docstrings.Get("autoscale")

	cmd := BuildCommandKS(nil, nil, autoscaleStrings, client, requireSession, requireAppName)
	// cmd.Deprecated = "use `flyctl scale` instead"

	disableCmdStrings := docstrings.Get("autoscale.disable")
	disableCmd := BuildCommand(cmd, runDisableAutoscaling, disableCmdStrings.Usage, disableCmdStrings.Short, disableCmdStrings.Long, client, requireSession, requireAppName)
	disableCmd.Args = cobra.RangeArgs(0, 2)

	setCmdStrings := docstrings.Get("autoscale.set")
	setCmd := BuildCommand(cmd, runSetParams, setCmdStrings.Usage, setCmdStrings.Short, setCmdStrings.Long, client, requireSession, requireAppName)
	setCmd.Args = cobra.RangeArgs(0, 2)

	showCmdStrings := docstrings.Get("autoscale.show")
	BuildCommand(cmd, runAutoscalingShow, showCmdStrings.Usage, showCmdStrings.Short, showCmdStrings.Long, client, requireSession, requireAppName)

	return cmd
}

func runSetParams(commandContext *cmdctx.CmdContext) error {
	return actualScale(commandContext, false)
}

func runDisableAutoscaling(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	newcfg := api.UpdateAutoscaleConfigInput{AppID: cmdCtx.AppName, Enabled: api.BoolPointer(false)}

	cfg, err := cmdCtx.Client.API().UpdateAutoscaleConfig(ctx, newcfg)
	if err != nil {
		return err
	}

	printScaleConfig(cmdCtx, cfg)

	return nil
}

func actualScale(cmdCtx *cmdctx.CmdContext, balanceRegions bool) error {
	ctx := cmdCtx.Command.Context()

	currentcfg, err := cmdCtx.Client.API().AppAutoscalingConfig(ctx, cmdCtx.AppName)
	if err != nil {
		return err
	}

	newcfg := api.UpdateAutoscaleConfigInput{AppID: cmdCtx.AppName}

	newcfg.BalanceRegions = &balanceRegions
	newcfg.MinCount = &currentcfg.MinCount
	newcfg.MaxCount = &currentcfg.MaxCount

	kvargs := make(map[string]string)

	for _, pair := range cmdCtx.Args {
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

	cfg, err := cmdCtx.Client.API().UpdateAutoscaleConfig(ctx, newcfg)
	if err != nil {
		return err
	}

	printScaleConfig(cmdCtx, cfg)

	return nil
}

func runAutoscalingShow(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	cfg, err := cmdCtx.Client.API().AppAutoscalingConfig(ctx, cmdCtx.AppName)
	if err != nil {
		return err
	}

	printScaleConfig(cmdCtx, cfg)

	return nil
}

func printScaleConfig(cmdCtx *cmdctx.CmdContext, cfg *api.AutoscalingConfig) {
	asJSON := cmdCtx.OutputJSON()

	if asJSON {
		cmdCtx.WriteJSON(cfg)
	} else {
		var mode string

		if !cfg.Enabled {
			mode = "Disabled"
		} else {
			mode = "Enabled"
		}

		fmt.Fprintf(cmdCtx.Out, "%15s: %s\n", "Autoscaling", mode)
		if cfg.Enabled {
			fmt.Fprintf(cmdCtx.Out, "%15s: %d\n", "Min Count", cfg.MinCount)
			fmt.Fprintf(cmdCtx.Out, "%15s: %d\n", "Max Count", cfg.MaxCount)
		}
	}
}
