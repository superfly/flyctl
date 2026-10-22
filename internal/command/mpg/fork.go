package mpg

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/uiex/mpg"
)

func newFork() *cobra.Command {
	const (
		long = `Fork a Managed Postgres cluster into a new cluster from the source's current
data, leaving the source cluster unchanged. The fork is provisioned
asynchronously in the same organization and billed separately.

The fork uses the source cluster's plan unless --plan picks a different one,
including a smaller one.`
		short = "Fork MPG cluster into a new cluster from its current data."
		usage = "fork [CLUSTER_ID]"
	)

	cmd := command.New(usage, short, long, runFork,
		command.RequireSession,
		requireMacaroonToken,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of the forked cluster (defaults to <source-name>-fork)",
		},
		flag.String{
			Name:        "plan",
			Description: "The plan for the forked cluster: " + planNames() + " (defaults to the source cluster's plan)",
		},
	)

	return cmd
}

func runFork(ctx context.Context) error {
	clusterID := flag.FirstArg(ctx)
	name := flag.GetString(ctx, "name")
	// The API is the source of truth for plans and matches them
	// case-insensitively, so --plan is passed through rather than checked
	// against MPGPlans. Leaving it empty keeps the source cluster's plan.
	plan := strings.TrimSpace(flag.GetString(ctx, "plan"))

	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}
	if cluster.Version == mpg.VersionV1 {
		return fmt.Errorf("forking is not supported for this cluster")
	}

	return cmdv2.RunFork(ctx, cluster.Id, name, plan)
}
