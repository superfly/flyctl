package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	"github.com/superfly/flyctl/internal/flag"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
)

func newConnect() (cmd *cobra.Command) {
	const (
		long = `Connect to a MPG database using psql`

		short = long
		usage = "connect <CLUSTER ID>"
	)

	cmd = command.New(usage, short, long, runConnect, command.RequireSession)

	flag.Add(cmd,
		flag.String{
			Name:        "database",
			Shorthand:   "d",
			Description: "The database to connect to",
		},
		flag.String{
			Name:        "username",
			Shorthand:   "u",
			Description: "The username to connect as",
		},
	)
	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runConnect(ctx context.Context) (err error) {
	clusterID := flag.FirstArg(ctx)
	var orgSlug string

	if clusterID != "" {
		// If cluster ID is provided, fetch directly without prompting for org
		mpgClient := mpgv1.ClientFromContext(ctx)
		response, err := mpgClient.GetManagedClusterById(ctx, clusterID)
		if err != nil {
			return fmt.Errorf("failed retrieving cluster %s: %w", clusterID, err)
		}
		orgSlug = response.Data.Organization.Slug
	} else {
		// Otherwise, prompt for org/cluster selection
		cluster, resolvedOrgSlug, err := ClusterFromArgOrSelect(ctx, clusterID, "")
		if err != nil {
			return err
		}
		clusterID = cluster.Id
		orgSlug = resolvedOrgSlug
	}

	// Resolve org slug alias for wireguard tunnel
	resolvedOrgSlug, err := AliasedOrganizationSlug(ctx, orgSlug)
	if err != nil {
		return fmt.Errorf("failed to resolve organization slug: %w", err)
	}

	return cmdv1.RunConnect(ctx, clusterID, resolvedOrgSlug)
}
