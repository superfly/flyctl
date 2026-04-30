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
		long  = `Show status and details of a specific Managed Postgres cluster using its ID.`
		short = "Show MPG cluster status."
		usage = "status [CLUSTER_ID]"
	)

	cmd := command.New(usage, short, long, runStatus,
		command.RequireSession,
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
