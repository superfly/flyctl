package tigris

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
)

func update() (cmd *cobra.Command) {
	const (
		short = "Update an existing Tigris object storage bucket"
		long  = short + "\n"
	)

	cmd = command.New("update <bucket_name>", short, long, runUpdate, command.RequireSession, command.LoadAppNameIfPresent)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
		extensions_core.SharedFlags,

		flag.Bool{
			Name:        "clear-shadow",
			Description: "Remove an existing shadow bucket",
		},

		flag.Bool{
			Name:        "private",
			Description: "Set a public bucket to be private",
		},
		SharedFlags,
	)
	return cmd
}

func runUpdate(ctx context.Context) (err error) {
	client := client.FromContext(ctx).API().GenqClient

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

	accessKey := flag.GetString(ctx, "shadow-access-key")
	secretKey := flag.GetString(ctx, "shadow-secret-key")
	region := flag.GetString(ctx, "shadow-region")
	shadowName := flag.GetString(ctx, "shadow-name")
	endpoint := flag.GetString(ctx, "shadow-endpoint")
	writeThrough := flag.GetBool(ctx, "shadow-write-through")
	clearShadow := flag.GetBool(ctx, "clear-shadow")

	// Check for shadow bucket values
	shadowBucketSpecified, err := isShadowBucketSpecified(accessKey, secretKey, region, shadowName, endpoint)
	if err != nil {
		return err
	}

	if clearShadow && shadowBucketSpecified {
		return fmt.Errorf("You cannot specify both --clear-shadow-bucket and shadow bucket fields")
	}

	if clearShadow {
		options["shadow_bucket"] = map[string]interface{}{}
	} else if shadowBucketSpecified {
		options["shadow_bucket"] = map[string]interface{}{
			"access_key":    accessKey,
			"secret_key":    secretKey,
			"region":        region,
			"name":          shadowName,
			"endpoint":      endpoint,
			"write_through": writeThrough,
		}
	}

	if flag.IsSpecified(ctx, "private") {
		options["public"] = false
	} else if flag.IsSpecified(ctx, "public") {
		options["public"] = flag.GetBool(ctx, "public")
	}

	if flag.IsSpecified(ctx, "no-accelerate") {
		options["accelerate"] = false
	} else if flag.IsSpecified(ctx, "accelerate") {
		options["accelerate"] = flag.GetBool(ctx, "accelerate")
	}

	_, err = gql.UpdateAddOn(ctx, client, addOn.Id, addOn.AddOnPlan.Id, []string{}, options)
	if err != nil {
		return
	}

	err = runStatus(ctx)
	return err
}
