package tigris

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
)

func list() (cmd *cobra.Command) {
	const (
		long  = `List your Tigris object storage buckets`
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
		out    = iostreams.FromContext(ctx).Out
		client = flyutil.ClientFromContext(ctx).GenqClient()
	)

	org, err := getOrg(ctx)
	if err != nil {
		return fmt.Errorf("error getting organization: %w", err)
	}

	var nodes []*gql.ListAddOnData
	if org != nil {
		response, err := gql.ListAddOnsForOrganization(ctx, client, "tigris", org.ID)
		if err != nil {
			return fmt.Errorf("error listing add-ons for organization: %w", err)
		}
		for _, node := range response.Organization.AddOns.Nodes {
			nodes = append(nodes, &node.ListAddOnData)
		}
	} else {
		response, err := gql.ListAddOns(ctx, client, "tigris")
		if err != nil {
			return fmt.Errorf("error listing add-ons: %w", err)
		}
		for _, node := range response.AddOns.Nodes {
			nodes = append(nodes, &node.ListAddOnData)
		}
	}

	var rows [][]string

	for _, extension := range nodes {
		rows = append(rows, []string{
			extension.Name,
			extension.Organization.Slug,
		})
	}

	_ = render.Table(out, "", rows, "Name", "Org")

	return
}

func getOrg(ctx context.Context) (*fly.Organization, error) {
	client := flyutil.ClientFromContext(ctx)

	orgName := flag.GetOrg(ctx)

	if orgName == "" {
		return nil, nil
	}

	return client.GetOrganizationBySlug(ctx, orgName)
}
