package tigris

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
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

		flag.String{
			Name:        "custom-domain",
			Description: "A custom domain name pointing at your bucket",
			Hidden:      true,
		},

		flag.Bool{
			Name:        "clear-shadow",
			Description: "Remove an existing shadow bucket",
		},

		flag.Bool{
			Name:        "clear-custom-domain",
			Description: "Remove a custom domain from a bucket",
			Hidden:      true,
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
	io := iostreams.FromContext(ctx)

	client := flyutil.ClientFromContext(ctx).GenqClient()

	id := flag.FirstArg(ctx)
	response, err := gql.GetAddOn(ctx, client, id, string(gql.AddOnTypeTigris))
	if err != nil {
		return
	}
	addOn := response.AddOn

	options, _ := addOn.Options.(map[string]interface{})

	if options == nil {
		options = make(map[string]interface{})
	}

	metadata, _ := addOn.Options.(map[string]interface{})

	if metadata == nil {
		metadata = make(map[string]interface{})
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

	if flag.IsSpecified(ctx, "custom-domain") {
		domain := flag.GetString(ctx, "custom-domain")

		if domain != addOn.Name {
			return fmt.Errorf("The custom domain must match the bucket name: %s != %s", domain, addOn.Name)
		}
		fmt.Fprintf(io.Out, "Before continuing, set a DNS CNAME record to enable your custom domain: %s -> %s\n\n", domain, addOn.Name+".fly.storage.tigris.dev")

		confirm, err := prompt.Confirm(ctx, "Continue with the update?")

		if err != nil || !confirm {
			return err
		}

		options["website"] = map[string]interface{}{
			"domain_name": domain,
		}
	}

	if flag.GetBool(ctx, "clear-custom-domain") {
		options["website"] = map[string]interface{}{
			"domain_name": "",
		}
	}

	_, err = gql.UpdateAddOn(ctx, client, addOn.Id, addOn.AddOnPlan.Id, []string{}, options, metadata)
	if err != nil {
		return
	}

	err = runStatus(ctx)
	return err
}
