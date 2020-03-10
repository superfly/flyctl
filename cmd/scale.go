package cmd

import (
	"fmt"
	"os"
	"regexp"
	"strconv"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
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

	regionCmdStrings := docstrings.Get("scale.regions")
	regionsCmd := BuildCommand(cmd, runScaleRegions, regionCmdStrings.Usage, regionCmdStrings.Short, regionCmdStrings.Long, true, os.Stdout, requireAppName)
	regionsCmd.AddBoolFlag(BoolFlagOpts{
		Name:        "reset-all",
		Description: "Reset all regions before applying changes",
	})
	regionsCmd.AddStringSliceFlag(StringSliceFlagOpts{
		Name:        "reset-region",
		Shorthand:   "x",
		Description: "Reset a region. Can be specified multiple times.",
	})

	vmCmdStrings := docstrings.Get("scale.vm")
	vmCmd := BuildCommand(cmd, runScaleVM, vmCmdStrings.Usage, vmCmdStrings.Short, vmCmdStrings.Long, true, os.Stdout, requireAppName)
	vmCmd.Args = cobra.RangeArgs(0, 1)

	return cmd
}

func runScaleRegions(ctx *CmdContext) error {
	input := api.UpdateAutoscaleConfigInput{
		AppID:   ctx.AppName,
		Regions: []api.AutoscaleRegionConfigInput{},
	}

	if ctx.Config.GetBool("reset-all") {
		input.ResetRegions = api.BoolPointer(true)
	}

	pattern := regexp.MustCompile("^(?P<region>[a-z]{3})(=(?P<count>\\d+))?(@(?P<weight>\\d+))?$")

	for _, pair := range ctx.Args {
		if !pattern.MatchString(pair) {
			return fmt.Errorf("Argument '%s' is invalid", pair)
		}

		names := pattern.SubexpNames()
		region := api.AutoscaleRegionConfigInput{}

		for idx, match := range pattern.FindStringSubmatch(pair) {
			if len(match) == 0 {
				continue
			}

			switch names[idx] {
			case "region":
				region.Code = match
			case "count":
				val, err := strconv.Atoi(match)
				if err != nil || val < 0 {
					return fmt.Errorf("Counts must be 0 or more (%s is invalid)", pair)
				}
				region.MinCount = api.IntPointer(val)
			case "weight":
				val, err := strconv.Atoi(match)
				if err != nil || val < 0 || val > 100 {
					return fmt.Errorf("Weights must be between 0 and 100 (%s is invalid)", pair)
				}
				region.Weight = api.IntPointer(val)
			}
		}

		input.Regions = append(input.Regions, region)
	}

	for _, region := range ctx.Config.GetStringSlice("reset-region") {
		input.Regions = append(input.Regions, api.AutoscaleRegionConfigInput{
			Code:  region,
			Reset: api.BoolPointer(true),
		})
	}

	if len(input.Regions) > 0 || input.ResetRegions != nil {
		fmt.Println("Updating autoscaling config...")
		config, err := ctx.FlyClient.UpdateAutoscaleConfig(input)

		if err != nil {
			return err
		}

		return renderAutoscalingConfig(ctx, config)
	}

	config, err := ctx.FlyClient.AppAutoscalingConfig(ctx.AppName)

	if err != nil {
		return err
	}

	return renderAutoscalingConfig(ctx, config)
}

func renderAutoscalingConfig(cc *CmdContext, config *api.AutoscalingConfig) error {
	fmt.Println(aurora.Bold("Autoscaling"))

	pairs := [][]string{
		[]string{"Enabled", fmt.Sprintf("%t", config.Enabled)},
		[]string{"Balance Regions", fmt.Sprintf("%t", config.BalanceRegions)},
	}
	if config.MinCount > 0 {
		pairs = append(pairs, []string{"Min Count", fmt.Sprintf("%d", config.MinCount)})
	}
	if config.MaxCount > 0 {
		pairs = append(pairs, []string{"Max Count", fmt.Sprintf("%d", config.MaxCount)})
	}

	printDefinitionList(pairs)

	if len(config.Regions) > 0 {
		fmt.Println()
		fmt.Println(aurora.Bold("Regions"))
		cc.Render(&presenters.AutoscalingRegionConfigs{Regions: config.Regions})
	}

	return nil
}

func runScaleVM(ctx *CmdContext) error {
	if len(ctx.Args) == 0 {
		size, err := ctx.FlyClient.AppVMSize(ctx.AppName)
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

	size, err := ctx.FlyClient.SetAppVMSize(ctx.AppName, sizeName)
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

// func renderVMSize(size api.VMSize) {
// 	pairs := [][]string{
// 		[]string{"Size", fmt.Sprintf("%t", config.Enabled)},
// 		[]string{"Balance Regions", fmt.Sprintf("%t", config.BalanceRegions)},
// 	}
// 	if config.MinCount > 0 {
// 		pairs = append(pairs, []string{"Min Count", fmt.Sprintf("%d", config.MinCount)})
// 	}
// 	if config.MaxCount > 0 {
// 		pairs = append(pairs, []string{"Max Count", fmt.Sprintf("%d", config.MaxCount)})
// 	}

// 	printDefinitionList(pairs)

// }
