package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/docstrings"

	"github.com/spf13/cobra"
)

func newAutoscaleCommand() *Command {
	autoscaleStrings := docstrings.Get("autoscale")

	cmd := &Command{
		Command: &cobra.Command{
			Use:   autoscaleStrings.Usage,
			Short: autoscaleStrings.Short,
			Long:  autoscaleStrings.Long,
		},
	}

	configureStrings := docstrings.Get("autoscale.configure")
	configCmd := BuildCommand(cmd, runAutoscaleConfigure, configureStrings.Usage, configureStrings.Short, configureStrings.Long, os.Stdout, requireSession, requireAppName)
	configCmd.AddIntFlag(IntFlagOpts{
		Name:        "min-count",
		Description: "The minimum number of instances to run",
	})
	configCmd.AddIntFlag(IntFlagOpts{
		Name:        "max-count",
		Description: "The maximum number of instances to run",
	})
	configCmd.AddBoolFlag(BoolFlagOpts{
		Name:        "balance-regions",
		Description: "Enable or disable moving vms as traffic patterns change",
	})

	showStrings := docstrings.Get("autoscale.show")
	BuildCommand(cmd, runAutoscaleShow, showStrings.Usage, showStrings.Short, showStrings.Long, os.Stdout, requireSession, requireAppName)

	return cmd
}

func runAutoscaleConfigure(ctx *CmdContext) error {
	currentcfg, err := ctx.Client.API().AppAutoscalingConfig(ctx.AppName)

	input := api.UpdateAutoscaleConfigInput{AppID: ctx.AppName}
	if ctx.Config.IsSet("min-count") {
		input.MinCount = api.IntPointer(ctx.Config.GetInt("min-count"))
	} else {
		input.MinCount = &currentcfg.MinCount
	}

	if ctx.Config.IsSet("max-count") {
		input.MaxCount = api.IntPointer(ctx.Config.GetInt("max-count"))
	} else {
		input.MaxCount = &currentcfg.MaxCount
	}

	if ctx.Config.IsSet("balance-regions") {
		input.BalanceRegions = api.BoolPointer(ctx.Config.GetBool("balance-regions"))
	} else {
		input.BalanceRegions = &currentcfg.BalanceRegions
	}

	cfg, err := ctx.Client.API().UpdateAutoscaleConfig(input)
	if err != nil {
		return err
	}

	printAutoscaleConfig(ctx.Out, cfg)

	return nil
}

func runAutoscaleShow(ctx *CmdContext) error {
	cfg, err := ctx.Client.API().AppAutoscalingConfig(ctx.AppName)
	if err != nil {
		return err
	}

	printAutoscaleConfig(ctx.Out, cfg)

	return nil
}

func printAutoscaleConfig(w io.Writer, cfg *api.AutoscalingConfig) {
	fmt.Fprintln(w, "Balance Regions:", cfg.BalanceRegions)
	fmt.Fprintln(w, "Min Count:", cfg.MinCount)
	fmt.Fprintln(w, "Max Count:", cfg.MaxCount)
}
