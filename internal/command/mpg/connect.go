package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/uiex/mpg"
)

const (
	localProxyPort = "16380"
)

func newConnect() (cmd *cobra.Command) {
	const (
		long = `Open a psql session to a Managed Postgres database. This starts a proxy
to the cluster over your organization's WireGuard network on local port
16380 and runs psql against it, so psql must be installed and on your PATH.

With no cluster ID, you pick an organization and a cluster from a list in
a terminal; without a terminal, the cluster ID is required. In a terminal,
you also pick the user and database unless you pass --username and
--database; otherwise the cluster's default credentials and database are
used.

Press Ctrl+C to cancel a running query, and Ctrl+C twice within two
seconds to end the session. To connect with another client, use
` + "`fly mpg proxy`" + `.`

		short = "Connect to a Managed Postgres database with psql"
		usage = "connect <CLUSTER ID>"
	)

	cmd = command.New(usage, short, long, runConnect, command.RequireSession, requireMacaroonToken)

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

	cluster, orgSlug, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}

	// Resolve org slug alias for wireguard tunnel
	resolvedOrgSlug, err := AliasedOrganizationSlug(ctx, orgSlug)
	if err != nil {
		return fmt.Errorf("failed to resolve organization slug: %w", err)
	}

	if cluster.Version == mpg.VersionV1 {
		return cmdv1.RunConnect(ctx, cluster.Id, resolvedOrgSlug)
	}

	return cmdv2.RunConnect(ctx, cluster.Id, resolvedOrgSlug, localProxyPort)
}
