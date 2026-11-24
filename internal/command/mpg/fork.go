package mpg

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiex/mpg"
)

func newFork() *cobra.Command {
	const (
		long = `Fork a Managed Postgres cluster into a new cluster from the source's current
data, leaving the source cluster unchanged. The fork is provisioned
asynchronously in the same organization and billed separately.

The fork uses the source cluster's plan unless --plan picks a different one,
including a smaller one. The command returns once the fork starts and
prints the new cluster's ID; check on it with ` + "`fly mpg status`" + `.

With no cluster ID, you pick an organization (unless FLY_ORG is set) and a
cluster from lists in a terminal; without a terminal, the cluster ID is
required. Not available for v1 clusters.`
		short = "Fork a Managed Postgres cluster into a new cluster"
		usage = "fork [CLUSTER_ID]"
	)

	cmd := command.New(usage, short, long, runFork,
		requireMutationSession,
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

	addStdoutJSONFlag(cmd, "JSON output (id, name, organization, region, plan, status; progress on stderr)",
		`  out=$(fly mpg fork "$source_id" --json) || { echo "$out" >&2; exit 1; }
  cluster_id=$(printf '%s' "$out" | jq -er '.id | select(. != "")') || exit 1`)

	return cmd
}

func runFork(ctx context.Context) error {
	clusterID := flag.FirstArg(ctx)
	if config.FromContext(ctx).JSONOutput && clusterID == "" {
		return prompt.NonInteractiveError("CLUSTER_ID must be specified when using --json")
	}
	name := flag.GetString(ctx, "name")
	// The API is the source of truth for plans, so --plan isn't checked
	// against MPGPlans. A plan found there by key or name goes out as its
	// key, the value the API knows: --plan Dev-256 or Developer sends
	// dev-256. Any other --plan is passed through as typed. Leaving it
	// empty keeps the source cluster's plan.
	plan := strings.TrimSpace(flag.GetString(ctx, "plan"))
	if key, ok := planKey(plan); ok {
		plan = key
	}

	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}
	if cluster.Version == mpg.VersionV1 {
		return fmt.Errorf("forking is not supported for this cluster")
	}

	return cmdv2.RunFork(ctx, cluster.Id, name, plan)
}
