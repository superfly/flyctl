package vector

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/command/secrets"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
)

func create() (cmd *cobra.Command) {
	const (
		short = "Provision a Upstash Vector index"
		long  = short + "\n"
	)

	cmd = command.New("create", short, long, runCreate, command.RequireSession, command.LoadAppNameIfPresent)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
		flag.Region(),
		extensions_core.SharedFlags,
		SharedFlags,
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of your cluster",
		},
	)

	return cmd
}

func selectSimilarityFunction(ctx context.Context, defaultValue string) (function *SimilarityFunction, err error) {
	var options []string
	for _, function := range similarityFunctions {
		options = append(options, fmt.Sprintf("%s (%s)", function.Name, function.UseCases))
	}

	var index int
	if err = prompt.Select(ctx, &index, "Select a similarity function:", defaultValue, options...); err == nil {
		function = &similarityFunctions[index]
	}

	return
}

func selectEmbeddingModel(ctx context.Context, defaultValue string) (function *EmbeddingModel, err error) {
	var index int
	if err = prompt.Select(ctx, &index, "Select an embedding model:", defaultValue, embeddingModelOptions()...); err == nil {
		function = embeddingModelForOption(index)
	}

	return
}

// embeddingModelOptions lists the choices for the embedding model prompt:
// "None", then every model in order.
func embeddingModelOptions() []string {
	options := []string{"None - I will provide my own embeddings"}

	for _, model := range embeddingModels {
		options = append(options, model.Name)
	}

	return options
}

// embeddingModelForOption returns the model at index in embeddingModelOptions,
// or nil for "None".
func embeddingModelForOption(index int) *EmbeddingModel {
	if index == 0 {
		return nil
	}

	return &embeddingModels[index-1]
}

func runCreate(ctx context.Context) (err error) {
	appName := appconfig.NameFromContext(ctx)
	params := extensions_core.ExtensionParams{}

	if appName != "" {
		params.AppName = appName
	} else {
		org, err := orgs.OrgFromFlagOrSelect(ctx)
		if err != nil {
			return err
		}

		params.Organization = org
	}

	function, err := selectSimilarityFunction(ctx, "")

	if err != nil {
		return err
	}

	model, err := selectEmbeddingModel(ctx, "")

	if err != nil {
		return err
	}

	var defaultDimensionCount = 128

	var options = gql.AddOnOptions{
		"similarity_function": function.Identifier,
		"dimension_count":     &defaultDimensionCount,
	}

	if model != nil {
		options["embedding_model"] = model.Identifier
		options["dimension_count"] = model.Dimensions
	} else if err := prompt.Int(ctx, &defaultDimensionCount, "How many dimensions?", defaultDimensionCount, false); err != nil {
		return err
	}

	params.Options = options
	params.PlanID = "aaV829vaM022XhQG28182aBG" // PAYG is the only plan for now
	params.Provider = "upstash_vector"
	extension, err := extensions_core.ProvisionExtension(ctx, params)
	if err != nil {
		return err
	}

	if extension.SetsSecrets {
		err = secrets.DeploySecrets(ctx, extension.App.Name, secrets.DeploymentArgs{
			Stage:    false,
			Detach:   false,
			CheckDNS: true,
		})
	}

	return err
}
