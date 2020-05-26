package cmd

import (
	"fmt"
	"os"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/docstrings"

	"github.com/spf13/cobra"
)

func newRegionsCommand() *Command {
	regionsStrings := docstrings.Get("regions")

	cmd := &Command{
		Command: &cobra.Command{
			Use:   regionsStrings.Usage,
			Short: regionsStrings.Short,
			Long:  regionsStrings.Long,
		},
	}

	addStrings := docstrings.Get("regions.add")
	addCmd := BuildCommand(cmd, runRegionsAdd, addStrings.Usage, addStrings.Short, addStrings.Long, os.Stdout, requireSession, requireAppName)
	addCmd.Args = cobra.MinimumNArgs(1)

	removeStrings := docstrings.Get("regions.remove")
	removeCmd := BuildCommand(cmd, runRegionsRemove, removeStrings.Usage, removeStrings.Short, removeStrings.Long, os.Stdout, requireSession, requireAppName)
	removeCmd.Args = cobra.MinimumNArgs(1)

	replaceStrings := docstrings.Get("regions.replace")
	replaceCmd := BuildCommand(cmd, runRegionsReplace, replaceStrings.Usage, replaceStrings.Short, replaceStrings.Long, os.Stdout, requireSession, requireAppName)
	replaceCmd.Args = cobra.MinimumNArgs(1)

	listStrings := docstrings.Get("regions.list")
	BuildCommand(cmd, runRegionsList, listStrings.Usage, listStrings.Short, listStrings.Long, os.Stdout, requireSession, requireAppName)

	return cmd
}

func runRegionsAdd(ctx *CmdContext) error {
	input := api.ConfigureRegionsInput{
		AppID:        ctx.AppName,
		AllowRegions: ctx.Args,
	}

	regions, err := ctx.Client.API().ConfigureRegions(input)
	if err != nil {
		return err
	}

	printRegions(ctx, regions, false)

	return nil
}

func runRegionsRemove(ctx *CmdContext) error {
	input := api.ConfigureRegionsInput{
		AppID:       ctx.AppName,
		DenyRegions: ctx.Args,
	}

	regions, err := ctx.Client.API().ConfigureRegions(input)
	if err != nil {
		return err
	}

	printRegions(ctx, regions, false)

	return nil
}

func runRegionsReplace(ctx *CmdContext) error {
	addList := make([]string, 0)
	delList := make([]string, 0)

	// Get the Region List
	regions, err := ctx.Client.API().ListAppRegions(ctx.AppName)
	if err != nil {
		return err
	}

	for _, r := range ctx.Args {
		found := false
		for _, er := range regions {
			if r == er.Code {
				found = true
				break
			}
		}
		if !found {
			addList = append(addList, r)
		}
	}

	for _, er := range regions {
		found := false
		for _, r := range ctx.Args {
			if r == er.Code {
				found = true
				break
			}
		}
		if !found {
			delList = append(delList, er.Code)
		}
	}

	input := api.ConfigureRegionsInput{
		AppID:        ctx.AppName,
		AllowRegions: addList,
		DenyRegions:  delList,
	}

	newregions, err := ctx.Client.API().ConfigureRegions(input)
	if err != nil {
		return err
	}

	printRegions(ctx, newregions, false)

	return nil
}

func runRegionsList(ctx *CmdContext) error {
	regions, err := ctx.Client.API().ListAppRegions(ctx.AppName)
	if err != nil {
		return err
	}

	printRegions(ctx, regions, true)

	return nil
}

func printRegions(ctx *CmdContext, regions []api.Region, verbose bool) {

	fmt.Println("Current Region Pool:")

	for _, r := range regions {
		if verbose {
			fmt.Printf("  %s  %s\n", r.Code, r.Name)
		} else {
			fmt.Printf("%s ", r.Code)
		}
	}

	fmt.Println()

}
