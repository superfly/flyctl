package vector

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func list() (cmd *cobra.Command) {
	const (
		long  = `List your Upstash Vector index`
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
	apiClient := flyutil.ClientFromContext(ctx)
	client := apiClient.GenqClient()
	orgSlug := flag.GetOrg(ctx)

	var rows [][]string
	if orgSlug != "" {
		if _, err := apiClient.GetOrganizationBySlug(ctx, orgSlug); err != nil {
			return err
		}

		response, err := gql.ListOrganizationAddOns(ctx, client, orgSlug, "upstash_vector")
		if err != nil {
			return err
		}

		for _, extension := range response.Organization.AddOns.Nodes {
			rows = append(rows, []string{extension.Name, orgSlug, extension.PrimaryRegion})
		}
	} else {
		response, err := gql.ListAddOns(ctx, client, "upstash_vector")
		if err != nil {
			return err
		}

		for _, extension := range response.AddOns.Nodes {
			rows = append(rows, []string{extension.Name, extension.Organization.Slug, extension.PrimaryRegion})
		}
	}

	out := iostreams.FromContext(ctx).Out
	_ = render.Table(out, "", rows, "Name", "Org", "Region")

	return nil
}
