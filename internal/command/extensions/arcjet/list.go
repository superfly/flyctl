package arcjet

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
)

func list() (cmd *cobra.Command) {
	const (
		long  = `List your Arcjet sites`
		short = long
		usage = "list"
	)

	cmd = command.New(usage, short, long, runList, command.RequireSession)

	cmd.Aliases = []string{"ls"}

	flag.Add(cmd,
		flag.Org(),
		extensions_core.SharedFlags,
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

	if orgSlug != "" {
		if _, err := apiClient.GetOrganizationBySlug(ctx, orgSlug); err != nil {
			return err
		}

		response, err := gql.ListOrganizationAddOns(ctx, client, orgSlug, gql.AddOnTypeArcjet)
		if err != nil {
			return err
		}

		for _, addon := range response.Organization.AddOns.Nodes {
			rows = append(rows, []string{addon.Name, orgSlug, addon.PrimaryRegion})
		}
	} else {
		response, err := gql.ListAddOns(ctx, client, gql.AddOnTypeArcjet)
		if err != nil {
			return err
		}

		for _, addon := range response.AddOns.Nodes {
			rows = append(rows, []string{addon.Name, addon.Organization.Slug, addon.PrimaryRegion})
		}
	}

	_ = render.Table(out, "", rows, "Name", "Org")

	return
}
