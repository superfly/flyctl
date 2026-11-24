package mpg

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/uiex/mpg"
)

func newStatus() *cobra.Command {
	const (
		long = `Show the status of a Managed Postgres cluster: its ID, name,
organization, region, status, storage used and allocated (only allocated
for v1 clusters), replica count and direct IP address. With --json, the
output includes the cluster's credentials, including the password.

With no cluster ID, you pick an organization (unless FLY_ORG is set) and a
cluster from lists in a terminal; without a terminal, the cluster ID is
required.`
		short = "Show the status of a Managed Postgres cluster"
		usage = "status [CLUSTER_ID]"
	)

	cmd := command.New(usage, short, long, runStatus,
		command.RequireSession,
		requireMacaroonToken,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd, flag.JSONOutput())

	return cmd
}

func runStatus(ctx context.Context) error {
	clusterID := flag.FirstArg(ctx)
	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}

	if cluster.Version == mpg.VersionV1 {
		return cmdv1.RunStatus(ctx, cluster.Id)

	}

	return cmdv2.RunStatus(ctx, cluster.Id)
}
