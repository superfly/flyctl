package kafka

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
)

func dashboard() (cmd *cobra.Command) {
	const (
		long = `Visit the Upstash Kafka dashboard on the Upstash web console`

		short = long
		usage = "dashboard"
	)

	cmd = command.New(usage, short, long, runDashboard, command.RequireSession, command.LoadAppNameIfPresent)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
		extensions_core.SharedFlags,
	)
	cmd.Args = cobra.NoArgs
	return cmd
}

func runDashboard(ctx context.Context) (err error) {
	if org := flag.GetOrg(ctx); org != "" {
		return extensions_core.OpenOrgDashboard(ctx, org, "upstash_kafka")
	}

	extension, _, err := extensions_core.Discover(ctx, gql.AddOnTypeUpstashKafka)
	if err != nil {
		return err
	}
	return extensions_core.OpenDashboard(ctx, extension.Name, gql.AddOnTypeUpstashKafka)
}
