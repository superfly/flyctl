package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagnames"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
)

func newProxy() (cmd *cobra.Command) {
	const (
		long = `Proxy to a MPG database`

		short = long
		usage = "proxy <CLUSTER ID>"
	)

	cmd = command.New(usage, short, long, runProxy, command.RequireSession)

	flag.Add(cmd,
		flag.String{
			Name:        flagnames.BindAddr,
			Shorthand:   "b",
			Default:     "127.0.0.1",
			Description: "Local address to bind to",
		},
		flag.String{
			Name:        flagnames.LocalPort,
			Shorthand:   "p",
			Default:     "16380",
			Description: "Local port to proxy on",
		},
	)

	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runProxy(ctx context.Context) (err error) {
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

	return cmdv1.RunProxy(ctx, clusterID, resolvedOrgSlug)
}
