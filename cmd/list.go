package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/superfly/flyctl/cmdctx"

	"github.com/superfly/flyctl/docstrings"
)

func newListCommand() *Command {
	ks := docstrings.Get("list")

	listCmd := BuildCommandKS(nil, nil, ks, os.Stdout, requireSession)
	listCmd.Aliases = []string{"ls"}

	laks := docstrings.Get("list.apps")
	listAppsCmd := BuildCommandKS(listCmd, runListApps, laks, os.Stdout, requireSession)

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
	BuildCommandKS(listCmd, runListOrgs, loks, os.Stdout, requireSession)

	return listCmd
}

type appCondensed struct {
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
		commandContext.Status("list", cmdctx.SERROR, "Too many arguments - discarding excess")
	}

	orgSlug, _ := commandContext.Config.GetString("org")

	status, _ := commandContext.Config.GetString("status")

	apps, err := commandContext.Client.API().GetApps()
	if err != nil {
		return err
	}

	var filteredApps []appCondensed

	filteredApps = make([]appCondensed, 0)

	for i := range apps {
		saved := false

		if appPart != "" {
			saved = strings.Contains(apps[i].Name, appPart)
		} else {
			saved = true
		}

		if orgSlug != "" {
			saved = saved && orgSlug == apps[i].Organization.Slug
		}

		if status != "" {
			saved = saved && status == apps[i].Status
		}

		if saved {
			filteredApps = append(filteredApps, appCondensed{ID: apps[i].ID,
				Name:         apps[i].Name,
				Status:       apps[i].Status,
				Deployed:     apps[i].Deployed,
				Hostname:     apps[i].Hostname,
				Organization: apps[i].Organization.Slug})
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

func runListOrgs(commandContext *cmdctx.CmdContext) error {
	asJSON := commandContext.OutputJSON()

	orgs, err := commandContext.Client.API().GetOrganizations()

	if err != nil {
		return err
	}

	if asJSON {
		commandContext.WriteJSON(orgs)
		return nil
	}

	fmt.Fprintf(commandContext.Out, "%16s %-32s\n", "Short Name", "Full Name")

	for _, org := range orgs {
		fmt.Fprintf(commandContext.Out, "%16s %-32s\n", org.Slug, org.Name)
	}

	return nil
}
