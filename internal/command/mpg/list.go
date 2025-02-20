package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func newList() *cobra.Command {
	const (
		short = "List Managed Postgres clusters"
		long  = short + "\n"
		usage = "list"
	)

	cmd := command.New(usage, short, long, runList,
		command.RequireSession,
		command.RequireUiex,
	)

	cmd.Aliases = []string{"ls"}

	flag.Add(cmd,
		flag.Org(),
		flag.JSONOutput(),
	)

	return cmd
}

func runList(ctx context.Context) error {
	org, err := orgs.OrgFromFlagOrSelect(ctx)
	if err != nil {
		return err
	}

	// Get the raw org slug for the UI-ex request
	genqClient := flyutil.ClientFromContext(ctx).GenqClient()
	fullOrg, err := gql.GetOrganization(ctx, genqClient, org.Slug)
	if err != nil {
		return fmt.Errorf("failed fetching org: %w", err)
	}

	var (
		out        = iostreams.FromContext(ctx).Out
		uiexClient = uiexutil.ClientFromContext(ctx)
		cfg        = config.FromContext(ctx)
	)

	response, err := uiexClient.ListManagedClusters(ctx, fullOrg.Organization.RawSlug)
	if err != nil {
		return err
	}

	if len(response.Data) == 0 {
		fmt.Fprintln(out, "No Managed Postgres clusters found")
		return nil
	}

	if cfg.JSONOutput {
		return render.JSON(out, response.Data)
	}

	var rows [][]string
	for _, cluster := range response.Data {
		rows = append(rows, []string{
			cluster.Id,
			cluster.Name,
			cluster.Organization.Slug,
			cluster.Region,
			cluster.Status,
		})
	}

	return render.Table(out, "", rows, "ID", "Name", "Org", "Region", "Status")
}