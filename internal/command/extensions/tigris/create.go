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
		SharedFlags,
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of your bucket",
		},
	)
	return cmd
}

func runCreate(ctx context.Context) (err error) {
	bucketName := flag.GetString(ctx, "name")
	if err := validateBucketName(bucketName); err != nil {
		return err
	}

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
	shadowBucketSpecified, err := isShadowBucketSpecified(accessKey, secretKey, region, name, endpoint)
	if err != nil {
		return err
	}

	// Include 'shadow_bucket' if all values are provided
	if shadowBucketSpecified {
		options["shadow_bucket"] = map[string]interface{}{
			"access_key":    accessKey,
			"secret_key":    secretKey,
			"region":        region,
			"name":          name,
			"endpoint":      endpoint,
			"write_through": writeThrough,
		}
	}

	// Always include 'website' because the Tigris API expects it
	options["website"] = map[string]interface{}{
		"domain_name": "",
	}

	// Assign the options to params
	params.Options = options

	params.Provider = "tigris"
	extension, err := extensions_core.ProvisionExtension(ctx, params)
	if err != nil {
		return err
	}

	if extension.SetsSecrets {
		err = secrets.DeploySecrets(ctx, gql.ToAppCompact(*extension.App), secrets.DeploymentArgs{
			Stage:    false,
			Detach:   false,
			CheckDNS: true,
		})
	}

	return err
}

func isShadowBucketSpecified(accessKey, secretKey, region, name, endpoint string) (bool, error) {
	values := []string{accessKey, secretKey, region, name, endpoint}

	var n int
	for _, value := range values {
		if value != "" {
			n++
		}
	}

	if n > 0 && n < len(values) {
		return false, fmt.Errorf("You must set all required shadow bucket values: shadow-access-key, shadow-secret-key, shadow-region, shadow-name, shadow-endpoint")
	}
	return n == len(values), nil
}

// validateBucketName validates that the bucket name length is within the allowed range.
// Bucket names must be between 3 and 63 characters.
// See: https://www.tigrisdata.com/docs/buckets/bucket-rules/
func validateBucketName(name string) error {
	const bucketNamingRulesURL = "https://www.tigrisdata.com/docs/buckets/bucket-rules/"

	if name == "" {
		return nil // Empty name will be handled by the prompt or auto-generated
	}

	if len(name) < 3 {
		return fmt.Errorf("bucket name %q is too short: must be at least 3 characters, got %d. See %s", name, len(name), bucketNamingRulesURL)
	}

	if len(name) > 63 {
		return fmt.Errorf("bucket name %q is too long: must be at most 63 characters, got %d. See %s", name, len(name), bucketNamingRulesURL)
	}

	return nil
}
