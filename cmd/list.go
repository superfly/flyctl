package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/docstrings"
)

func newListCommand() *Command {
	ks := docstrings.Get("list")

	listCmd := &Command{
		Command: &cobra.Command{
			Use:     ks.Usage,
			Aliases: []string{"ls"},
			Short:   ks.Short,
			Long:    ks.Long,
		},
	}

	laks := docstrings.Get("list.apps")
	listAppsCmd := BuildCommand(listCmd, runListApps, laks.Usage, laks.Short, laks.Long, os.Stdout)

	listAppsCmd.AddStringFlag(StringFlagOpts{
		Name:        "org",
		Shorthand:   "o",
		Description: `Show only apps in this organisation`,
	})

	listAppsCmd.AddStringFlag(StringFlagOpts{
		Name:        "status",
		Shorthand:   "s",
		Description: `Show only apps with this status`,
	})

	loks := docstrings.Get("list.orgs")
	BuildCommand(listCmd, runListOrgs, loks.Usage, loks.Short, loks.Long, os.Stdout)

	return listCmd
}

func runList(ctx *CmdContext) error {
	fmt.Fprintln(ctx.Out, "list can display apps (list apps) or orgs (list orgs)")
	return nil
}

func runListApps(ctx *CmdContext) error {

	appPart := ""

	if len(ctx.Args) == 1 {
		appPart = ctx.Args[0]
	} else if len(ctx.Args) > 0 {
		fmt.Fprintln(ctx.Out, "Too many arguments - discarding excess")
	}

	orgslug, _ := ctx.Config.GetString("org")

	status, _ := ctx.Config.GetString("status")

	apps, err := ctx.Client.API().GetApps()
	if err != nil {
		return err
	}

	fmt.Fprintf(ctx.Out, "%32s %10s %16s\n", "Name", "Status", "Organization")

	for _, app := range apps {
		print := false

		if appPart != "" {
			print = strings.Contains(app.Name, appPart)
		} else {
			print = true
		}

		if orgslug != "" {
			print = (print && orgslug == app.Organization.Slug)
		}

		if status != "" {
			print = (print && status == app.Status)
		}

		if print {
			fmt.Fprintf(ctx.Out, "%32s %10s %16s\n", app.Name, app.Status, app.Organization.Slug)
		}
	}

	return nil
}

func runListOrgs(ctx *CmdContext) error {
	orgs, err := ctx.Client.API().GetOrganizations()

	if err != nil {
		return err
	}

	fmt.Fprintf(ctx.Out, "%16s %-32s\n", "Short Name", "Full Name")

	for _, org := range orgs {
		fmt.Fprintf(ctx.Out, "%16s %-32s\n", org.Slug, org.Name)
	}

	return nil
}
