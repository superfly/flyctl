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
		short = "Provision an Upstash Vector index"
		long  = `Provision an Upstash Vector index on the pay-as-you-go plan. In an app
directory, or with ` + "`--app`" + `, the index is attached to that app, and the
command fails if the app already has one. Otherwise the index belongs to
an organization, which you pick from a list unless you pass ` + "`--org`" + `.

You pick a similarity function and an embedding model from lists. If you
provide your own embeddings, you're asked for the number of dimensions
(default 128). There are no flags for these choices, so this command
needs a terminal. The first time you use Upstash Vector you must also
accept its terms of service; ` + "`--yes`" + ` accepts them.

For an app's index, the credentials are set as secrets on the app and
its machines restart to pick them up. If any of those secrets already
exist, none are set, and the values are printed for you to set yourself.`
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
	var options []string

	options = append(options, "None - I will provide my own embeddings")

	for _, model := range embeddingModels {
		options = append(options, model.Name)
	}

	var index int
	if err = prompt.Select(ctx, &index, "Select an embedding model:", defaultValue, options...); err == nil {
		if index != 0 {
			function = &embeddingModels[index]
		}
	}

	return
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
	} else {
		prompt.Int(ctx, options["dimension_count"].(*int), "How many dimensions?", defaultDimensionCount, false)
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
