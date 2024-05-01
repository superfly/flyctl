package vector

import (
	"context"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
)

func update() (cmd *cobra.Command) {
	const (
		short = "Update an existing Upstash Vector index"
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
	client := fly.ClientFromContext(ctx).GenqClient

	id := flag.FirstArg(ctx)
	response, err := gql.GetAddOn(ctx, client, id)
	if err != nil {
		return
	}
	addOn := response.AddOn

	options, _ := addOn.Options.(map[string]interface{})
	if options == nil {
		options = make(map[string]interface{})
	}

	function, err := selectSimilarityFunction(ctx, "")

	if err != nil {
		return err
	}

	options["similarity_function"] = function.Identifier

	model, err := selectEmbeddingModel(ctx, "")

	if err != nil {
		return err
	}

	if model != nil {
		options["embedding_model"] = model.Identifier
		options["dimension_count"] = model.Dimensions
	} else {
		prompt.Int(ctx, options["dimension_count"].(*int), "How many dimensions?", options["dimension_count"].(int), false)
	}

	_, err = gql.UpdateAddOn(ctx, client, addOn.Id, addOn.AddOnPlan.Id, []string{}, options)
	if err != nil {
		return
	}
	return runStatus(ctx)
}
