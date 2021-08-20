package cmd

import (
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/client"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/docstrings"

	"github.com/spf13/cobra"
)

func newRegionsCommand(client *client.Client) *Command {
	regionsStrings := docstrings.Get("regions")

	cmd := BuildCommandKS(nil, nil, regionsStrings, client, nil, requireAppName, requireSession)

	addStrings := docstrings.Get("regions.add")
	addCmd := BuildCommandKS(cmd, runRegionsAdd, addStrings, client, nil, requireSession, requireAppName)
	addCmd.Args = cobra.MinimumNArgs(1)

	removeStrings := docstrings.Get("regions.remove")
	removeCmd := BuildCommandKS(cmd, runRegionsRemove, removeStrings, client, nil, requireSession, requireAppName)
	removeCmd.Args = cobra.MinimumNArgs(1)

	setStrings := docstrings.Get("regions.set")
	setCmd := BuildCommandKS(cmd, runRegionsSet, setStrings, client, nil, requireSession, requireAppName)
	setCmd.Args = cobra.MinimumNArgs(1)

	setBackupStrings := docstrings.Get("regions.backup")
	setBackupCmd := BuildCommand(cmd, runBackupRegionsSet, setBackupStrings.Usage, setBackupStrings.Short, setBackupStrings.Long, client, nil, requireSession, requireAppName)
	setBackupCmd.Args = cobra.MinimumNArgs(1)

	listStrings := docstrings.Get("regions.list")
	BuildCommand(cmd, runRegionsList, listStrings.Usage, listStrings.Short, listStrings.Long, client, nil, requireSession, requireAppName)

	return cmd
}

func runRegionsAdd(ctx *cmdctx.CmdContext) error {
	input := api.ConfigureRegionsInput{
		AppID:        ctx.AppName,
		AllowRegions: ctx.Args,
	}

	regions, backupRegions, err := ctx.Client.API().ConfigureRegions(input)
	if err != nil {
		return err
	}

	printRegions(ctx, regions, backupRegions)

	return nil
}

func runRegionsRemove(ctx *cmdctx.CmdContext) error {
	input := api.ConfigureRegionsInput{
		AppID:       ctx.AppName,
		DenyRegions: ctx.Args,
	}

	regions, backupRegions, err := ctx.Client.API().ConfigureRegions(input)
	if err != nil {
		return err
	}

	printRegions(ctx, regions, backupRegions)

	return nil
}

func runRegionsSet(ctx *cmdctx.CmdContext) error {
	addList := make([]string, 0)
	delList := make([]string, 0)

	// Get the Region List
	regions, _, err := ctx.Client.API().ListAppRegions(ctx.AppName)
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

	newregions, backupRegions, err := ctx.Client.API().ConfigureRegions(input)
	if err != nil {
		return err
	}

	printRegions(ctx, newregions, backupRegions)

	return nil
}

func runRegionsList(ctx *cmdctx.CmdContext) error {
	regions, backupRegions, err := ctx.Client.API().ListAppRegions(ctx.AppName)
	if err != nil {
		return err
	}

	printRegions(ctx, regions, backupRegions)

	return nil
}

func runBackupRegionsSet(ctx *cmdctx.CmdContext) error {
	input := api.ConfigureRegionsInput{
		AppID:         ctx.AppName,
		BackupRegions: ctx.Args,
	}

	regions, backupRegions, err := ctx.Client.API().ConfigureRegions(input)
	if err != nil {
		return err
	}

	printRegions(ctx, regions, backupRegions)

	return nil
}

func printRegions(ctx *cmdctx.CmdContext, regions []api.Region, backupRegions []api.Region) {

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

	if verbose {
		ctx.Status("backupRegions", cmdctx.STITLE, "Current Backup Region Pool:")
	} else {
		ctx.Status("backupRegions", cmdctx.STITLE, "Backup Region: ")
	}

	for _, r := range backupRegions {
		if verbose {
			ctx.Statusf("backupRegions", cmdctx.SINFO, "  %s  %s\n", r.Code, r.Name)
		} else {
			ctx.Status("backupRegions", cmdctx.SINFO, r.Code)
		}
	}
}
