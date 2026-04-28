package redis

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
)

func newList() (cmd *cobra.Command) {
	const (
		long  = `List Upstash Redis databases for an organization`
		short = long
		usage = "list"
	)

	cmd = command.New(usage, short, long, runList, command.RequireSession)

	cmd.Aliases = []string{"ls"}

	flag.Add(cmd,
		flag.Org(),
	)

	return cmd
}

func runList(ctx context.Context) (err error) {
	var (
		out       = iostreams.FromContext(ctx).Out
		apiClient = flyutil.ClientFromContext(ctx)
		client    = apiClient.GenqClient()
		orgSlug   = flag.GetOrg(ctx)
	)

	var rows [][]string
	appendRow := func(name, org, plan, primaryRegion string, readRegions []string, options map[string]any) {
		if options == nil {
			options = make(map[string]any)
		}
		eviction := "Disabled"
		if options["eviction"] != nil && options["eviction"].(bool) {
			eviction = "Enabled"
		}

		rows = append(rows, []string{name, org, plan, eviction, primaryRegion, strings.Join(readRegions, ",")})
	}

	if orgSlug != "" {
		if _, err := apiClient.GetOrganizationBySlug(ctx, orgSlug); err != nil {
			return err
		}

		response, err := gql.ListOrganizationAddOns(ctx, client, orgSlug, "upstash_redis")
		if err != nil {
			return err
		}

		for _, addon := range response.Organization.AddOns.Nodes {
			options, _ := addon.Options.(map[string]any)
			appendRow(addon.Name, orgSlug, addon.AddOnPlan.DisplayName, addon.PrimaryRegion, addon.ReadRegions, options)
		}
	} else {
		response, err := gql.ListAddOns(ctx, client, "upstash_redis")
		if err != nil {
			return err
		}

		for _, addon := range response.AddOns.Nodes {
			options, _ := addon.Options.(map[string]any)
			appendRow(addon.Name, addon.Organization.Slug, addon.AddOnPlan.DisplayName, addon.PrimaryRegion, addon.ReadRegions, options)
		}
	}

	_ = render.Table(out, "", rows, "Name", "Org", "Plan", "Eviction", "Primary Region", "Read Regions")

	return
}
