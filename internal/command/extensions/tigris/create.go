package tigris

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
)

func create() (cmd *cobra.Command) {

	const (
		short = "Provision a Tigris object storage bucket"
		long  = short + "\n"
	)

	cmd = command.New("create", short, long, runCreate, command.RequireSession, command.LoadAppNameIfPresent)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
		extensions_core.SharedFlags,

		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of your bucket",
		},
		flag.Bool{
			Name:        "public",
			Shorthand:   "p",
			Description: "Objects in the bucket should be publicly accessible",
		},
		flag.String{
			Name:        "shadow-access-key",
			Description: "Shadow bucket access key",
		},

		flag.String{
			Name:        "shadow-secret-key",
			Description: "Shadow bucket secret key",
		},
		flag.String{
			Name:        "shadow-region",
			Description: "Shadow bucket region",
		},
		flag.String{
			Name:        "shadow-endpoint",
			Description: "Shadow bucket endpoint",
		},
		flag.String{
			Name:        "shadow-name",
			Description: "Shadow bucket name",
		},
		flag.Bool{
			Name:        "shadow-write-through",
			Description: "Write objects through to the shadow bucket",
		},
		flag.Bool{
			Name:        "accelerate",
			Hidden:      true,
			Description: "Cache objects on write in all regions",
		},
		flag.String{
			Name:        "website-domain-name",
			Description: "Domain name for website",
			Hidden:      true,
		},
	)
	return cmd
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

	options := gql.AddOnOptions{}

	options["public"] = flag.GetBool(ctx, "public")
	options["accelerate"] = flag.GetBool(ctx, "accelerate")

	accessKey := flag.GetString(ctx, "shadow-access-key")
	secretKey := flag.GetString(ctx, "shadow-secret-key")
	region := flag.GetString(ctx, "shadow-region")
	name := flag.GetString(ctx, "shadow-name")

	endpoint := flag.GetString(ctx, "shadow-endpoint")
	writeThrough := flag.GetBool(ctx, "shadow-write-through")

	// Check for shadow bucket values
	shadowBucketValues := []string{accessKey, secretKey, region, name, endpoint}
	allProvided := true
	noneProvided := true

	for _, value := range shadowBucketValues {
		if value == "" {
			allProvided = false
		} else {
			noneProvided = false
		}
	}

	// Issue error if not all values are provided together
	if !allProvided && !noneProvided {
		return fmt.Errorf("You must set all required shadow bucket values: shadow-access-key, shadow-secret-key, shadow-region, shadow-name, shadow-endpoint")
	}

	// Include 'shadow_bucket' if all values are provided
	if allProvided {
		options["shadow_bucket"] = map[string]interface{}{
			"access_key":    accessKey,
			"secret_key":    secretKey,
			"region":        region,
			"name":          name,
			"endpoint":      endpoint,
			"write_through": writeThrough,
		}
	}

	// Always include 'website'
	options["website"] = map[string]interface{}{
		"domain_name": flag.GetString(ctx, "website-domain-name"),
	}

	// Assign the options to params
	params.Options = options

	params.Provider = "tigris"
	extension, err := extensions_core.ProvisionExtension(ctx, params)

	if err != nil {
		return err
	}

	if extension.SetsSecrets {
		err = secrets.DeploySecrets(ctx, gql.ToAppCompact(*extension.App), false, false)
	}

	return err
}
