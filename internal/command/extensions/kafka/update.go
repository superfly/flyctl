package kafka

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
)

func update() (cmd *cobra.Command) {
	const (
		short = "Update an existing Upstash Kafka cluster"
		long  = short + "\n"
	)

	cmd = command.New("update <name>", short, long, runUpdate, command.RequireSession, command.LoadAppNameIfPresent)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
		extensions_core.SharedFlags,
		SharedFlags,
	)
	return cmd
}

func runUpdate(ctx context.Context) (err error) {
	client := flyutil.ClientFromContext(ctx).GenqClient()

	id := flag.FirstArg(ctx)
	response, err := gql.GetAddOn(ctx, client, id, string(gql.AddOnTypeUpstashKafka))
	if err != nil {
		return
	}
	addOn := response.AddOn

	options, _ := addOn.Options.(map[string]interface{})
	if options == nil {
		options = make(map[string]interface{})
	}

	_, err = gql.UpdateAddOn(ctx, client, addOn.Id, addOn.AddOnPlan.Id, []string{}, options, addOn.Metadata)
	if err != nil {
		return
	}
	return runStatus(ctx)
}
