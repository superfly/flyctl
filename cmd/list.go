package cmd

import (
	"fmt"
	"github.com/superfly/flyctl/cmdctx"
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

func runList(ctx *cmdctx.CmdContext) error {
	fmt.Fprintln(ctx.Out, "list can display apps (list apps) or orgs (list orgs)")
	return nil
}

type CondensedApp struct {
	ID           string
	Name         string
	Status       string
	Deployed     bool
	Hostname     string
	Organization string
}

func runListApps(commandContext *cmdctx.CmdContext) error {
	asJSON := commandContext.OutputJSON()

	appPart := ""

	if len(commandContext.Args) == 1 {
		appPart = commandContext.Args[0]
	} else if len(commandContext.Args) > 0 {
		commandContext.Status("flyctl", cmdctx.SERROR, "Too many arguments - discarding excess")
	}

	orgSlug, _ := commandContext.Config.GetString("org")

	status, _ := commandContext.Config.GetString("status")

	apps, err := commandContext.Client.API().GetApps()
	if err != nil {
		return err
	}

	var filteredApps []CondensedApp

	filteredApps = make([]CondensedApp, 0)

	for _, app := range apps {
		saved := false

		if appPart != "" {
			saved = strings.Contains(app.Name, appPart)
		} else {
			saved = true
		}

		if orgSlug != "" {
			saved = saved && orgSlug == app.Organization.Slug
		}

		if status != "" {
			saved = saved && status == app.Status
		}

		if saved {
			filteredApps = append(filteredApps, CondensedApp{ID: app.ID, Name: app.Name, Status: app.Status, Deployed: app.Deployed, Hostname: app.Hostname, Organization: app.Organization.Slug})
		}
	}

	if asJSON {
		commandContext.WriteJSON(filteredApps)
	} else {
		fmt.Fprintf(commandContext.Out, "%32s %10s %16s\n", "Name", "Status", "Organization")

		for _, app := range filteredApps {
			fmt.Fprintf(commandContext.Out, "%32s %10s %16s\n", app.Name, app.Status, app.Organization)
		}
	}

	return nil
}

func runListOrgs(ctx *cmdctx.CmdContext) error {
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
