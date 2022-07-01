package cmd

import (
	"sort"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/client"

	"github.com/superfly/flyctl/docstrings"
)

func newListCommand(client *client.Client) *Command {
	ks := docstrings.Get("list")

	listCmd := BuildCommandKS(nil, nil, ks, client, requireSession)
	listCmd.Aliases = []string{"ls"}

	laks := docstrings.Get("list.apps")
	listAppsCmd := BuildCommandKS(listCmd, runListApps, laks, client, requireSession)

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

	listAppsCmd.AddBoolFlag(BoolFlagOpts{
		Name:        "exact",
		Shorthand:   "e",
		Description: `Show exact times`,
		Default:     false,
	})

	listAppsCmd.AddStringFlag(StringFlagOpts{
		Name:        "sort",
		Description: "Sort by name, created",
		Default:     "name",
	})

	loks := docstrings.Get("list.orgs")
	BuildCommandKS(listCmd, runListOrgs, loks, client, requireSession)

	return listCmd
}

type appCondensed struct {
	ID           string
	Name         string
	Status       string
	Deployed     bool
	Hostname     string
	Organization string
	CreatedAt    time.Time
}

func runListApps(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()
	asJSON := cmdCtx.OutputJSON()

	appPart := ""

	if len(cmdCtx.Args) == 1 {
		appPart = cmdCtx.Args[0]
	} else if len(cmdCtx.Args) > 0 {
		cmdCtx.Status("list", cmdctx.SERROR, "Too many arguments - discarding excess")
	}

	orgSlug := cmdCtx.Config.GetString("org")
	status := cmdCtx.Config.GetString("status")
	exact := cmdCtx.Config.GetBool("exact")

	apps, err := cmdCtx.Client.API().GetApps(ctx, nil)
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
			var createdAt time.Time

			// CreatedAt may or may not exist
			if apps[i].Deployed && apps[i].CurrentRelease != nil {
				createdAt = apps[i].CurrentRelease.CreatedAt
			}

			filteredApps = append(filteredApps, appCondensed{ID: apps[i].ID,
				Name:         apps[i].Name,
				Status:       apps[i].Status,
				Deployed:     apps[i].Deployed,
				Hostname:     apps[i].Hostname,
				Organization: apps[i].Organization.Slug,
				CreatedAt:    createdAt})
		}
	}

	sortType := cmdCtx.Config.GetString("sort")
	if err != nil {
		return err
	}

	switch sortType {
	case "created":
		sort.Slice(filteredApps, func(i, j int) bool { return filteredApps[i].CreatedAt.After(filteredApps[j].CreatedAt) })
	case "name":
		fallthrough
	default:
		sort.Slice(filteredApps, func(i, j int) bool { return filteredApps[i].Name < filteredApps[j].Name })
	}

	if asJSON {
		cmdCtx.WriteJSON(filteredApps)
		return nil
	}

	table := helpers.MakeSimpleTable(cmdCtx.Out, []string{"Name", "Status", "Org", "Deployed"})

	for _, a := range filteredApps {
		createdAt := ""
		if !a.CreatedAt.IsZero() {
			if exact {
				createdAt = a.CreatedAt.Format(time.RFC3339)
			} else {
				createdAt = humanize.Time(a.CreatedAt)
			}
		}

		table.Append([]string{a.Name, a.Status, a.Organization, createdAt})
	}

	table.Render()

	return nil
}

func runListOrgs(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	asJSON := cmdCtx.OutputJSON()

	orgs, err := cmdCtx.Client.API().GetOrganizations(ctx)

	if err != nil {
		return err
	}

	if asJSON {
		cmdCtx.WriteJSON(orgs)
		return nil
	}

	table := helpers.MakeSimpleTable(cmdCtx.Out, []string{"Name", "Slug", "Type"})

	sort.Slice(orgs, func(i, j int) bool { return orgs[i].Type < orgs[j].Type })

	for _, org := range orgs {
		table.Append([]string{org.Name, org.Slug, org.Type})
	}

	table.Render()

	return nil
}
