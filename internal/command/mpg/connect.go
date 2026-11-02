package mpg

import (
	"context"
	"fmt"
	"os/exec"

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
		long = `Connect to a MPG database using a locally installed psql client.

Install the PostgreSQL client tools and ensure psql is available in your PATH:
https://www.postgresql.org/download/`

		short = "Connect to a MPG database using psql"
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
	psqlPath, err := exec.LookPath("psql")
	if err != nil {
		return fmt.Errorf("psql is required to connect to MPG: %w\nInstall the PostgreSQL client tools and add their bin directory to PATH: https://www.postgresql.org/download/", err)
	}

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
		return cmdv1.RunConnect(ctx, cluster.Id, resolvedOrgSlug, psqlPath)
	}

	return cmdv2.RunConnect(ctx, cluster.Id, resolvedOrgSlug, localProxyPort, psqlPath)
}
