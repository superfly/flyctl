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

	fmt.Println("Allowed Regions:")
	for _, r := range regions {
		fmt.Printf("  %s  %s\n", r.Code, r.Name)
	}

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

	fmt.Println("Allowed Regions:")
	for _, r := range regions {
		fmt.Printf("  %s  %s\n", r.Code, r.Name)
	}

	return nil
}

func runRegionsList(ctx *CmdContext) error {
	regions, err := ctx.Client.API().ListAppRegions(ctx.AppName)
	if err != nil {
		return err
	}

	fmt.Println("Allowed Regions:")
	for _, r := range regions {
		fmt.Printf("  %s  %s\n", r.Code, r.Name)
	}

	return nil
}
