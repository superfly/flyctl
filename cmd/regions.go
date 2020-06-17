package cmd

import (
	"github.com/superfly/flyctl/cmdctx"
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

	setStrings := docstrings.Get("regions.set")
	setCmd := BuildCommand(cmd, runRegionsSet, setStrings.Usage, setStrings.Short, setStrings.Long, os.Stdout, requireSession, requireAppName)
	setCmd.Args = cobra.MinimumNArgs(1)

	listStrings := docstrings.Get("regions.list")
	BuildCommand(cmd, runRegionsList, listStrings.Usage, listStrings.Short, listStrings.Long, os.Stdout, requireSession, requireAppName)

	return cmd
}

func runRegionsAdd(ctx *cmdctx.CmdContext) error {
	input := api.ConfigureRegionsInput{
		AppID:        ctx.AppName,
		AllowRegions: ctx.Args,
	}

	regions, err := ctx.Client.API().ConfigureRegions(input)
	if err != nil {
		return err
	}

	printRegions(ctx, regions)

	return nil
}

func runRegionsRemove(ctx *cmdctx.CmdContext) error {
	input := api.ConfigureRegionsInput{
		AppID:       ctx.AppName,
		DenyRegions: ctx.Args,
	}

	regions, err := ctx.Client.API().ConfigureRegions(input)
	if err != nil {
		return err
	}

	printRegions(ctx, regions)

	return nil
}

func runRegionsSet(ctx *cmdctx.CmdContext) error {
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

	printRegions(ctx, newregions)

	return nil
}

func runRegionsList(ctx *cmdctx.CmdContext) error {
	regions, err := ctx.Client.API().ListAppRegions(ctx.AppName)
	if err != nil {
		return err
	}

	printRegions(ctx, regions)

	return nil
}

func printRegions(ctx *cmdctx.CmdContext, regions []api.Region) {

	if ctx.OutputJSON() {
		ctx.WriteJSON(regions)
		return
	}

	verbose := ctx.GlobalConfig.GetBool("verbose")

	if verbose {
		ctx.Status("regions", cmdctx.STITLE, "Current Region Pool:")
	} else {
		ctx.Status("regions", cmdctx.STITLE, "Region Pool: ")
	}

	for _, r := range regions {
		if verbose {
			ctx.Statusf("regions", cmdctx.SINFO, "  %s  %s\n", r.Code, r.Name)
		} else {
			ctx.Status("regions", cmdctx.SINFO, r.Code)
		}
	}

}
