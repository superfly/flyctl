package statics

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	extensions "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/haikunator"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/resset"
	"github.com/superfly/tokenizer"
)

// FindBucket finds the tigris statics bucket for the given app and org.
// Returns nil, nil if no bucket is found.
func FindBucket(ctx context.Context, app *fly.App, org *fly.Organization) (*gql.ListAddOnsAddOnsAddOnConnectionNodesAddOn, error) {

	client := flyutil.ClientFromContext(ctx)
	gqlClient := client.GenqClient()

	response, err := gql.ListAddOns(ctx, gqlClient, "tigris")
	if err != nil {
		return nil, err
	}

	// Using string comparison here because we might want to use BigInt app IDs in the future.
	internalAppIdStr := strconv.FormatUint(uint64(app.InternalNumericID), 10)

	for _, extension := range response.AddOns.Nodes {
		if extension.Metadata == nil {
			continue
		}
		if extension.Organization.Slug != org.Slug {
			continue
		}
		if extension.Metadata.(map[string]interface{})[staticsMetaKeyAppId] == internalAppIdStr {
			return &extension, nil
		}
	}
	return nil, nil
}

func (deployer *DeployerState) ensureBucketCreated(ctx context.Context) (tokenizedAuth string, retErr error) {

	client := flyutil.ClientFromContext(ctx)

	bucket, err := FindBucket(ctx, deployer.app, deployer.org)
	if err != nil {
		return "", err
	}
	if bucket != nil {
		deployer.bucket = bucket.Name
		return bucket.Metadata.(map[string]interface{})[staticsMetaTokenizedAuth].(string), nil
	}

	// Using string comparison here because we might want to use BigInt app IDs in the future.
	internalAppIdStr := strconv.FormatUint(uint64(deployer.app.InternalNumericID), 10)

	bucketName := fmt.Sprintf("%s-statics", deployer.appConfig.AppName)

	params := extensions.ExtensionParams{
		Organization:         deployer.org,
		Provider:             "tigris",
		Options:              gql.AddOnOptions{},
		ErrorCaptureCallback: nil,
		OverrideRegion:       deployer.appConfig.PrimaryRegion,
		OverrideName:         &bucketName,
	}
	params.Options["website"] = map[string]interface{}{
		"domain_name": "",
	}
	params.Options["accelerate"] = false
	// TODO(allison): Make sure we still need this when virtual services drop :)
	params.Options["public"] = true

	// TODO(allison): Make this quiet - it outputs credentials that we don't need to show.
	ext, err := extensions.ProvisionExtension(ctx, params)
	if err != nil {
		// If the extension name is taken, try again, haikunating the name.
		// If that fails too, return the original error. Otherwise, continue successfully
		if strings.Contains(err.Error(), "already exists for app") ||
			strings.Contains(err.Error(), "unavailable for creation") {
			bucketName = fmt.Sprintf("%s-%s", *params.OverrideName, haikunator.Haikunator().String())
			params.OverrideName = &bucketName
			newExt, newErr := extensions.ProvisionExtension(ctx, params)
			if newErr == nil {
				ext = newExt
				err = nil
			}
		}
	}
	if err != nil {
		return "", err
	}

	deployer.bucket = bucketName

	defer func() {
		if retErr != nil {
			client := flyutil.ClientFromContext(ctx).GenqClient()
			// Using context.Background() here in case the error is that the context is canceled.
			_, err := gql.DeleteAddOn(context.Background(), client, bucketName)
			if err != nil {
				fmt.Fprintf(iostreams.FromContext(ctx).ErrOut, "Failed to delete extension: %v\n", err)
			}
		}
	}()

	secrets := ext.Data.Environment.(map[string]interface{})

	tokenizedKey, err := deployer.tokenizeTigrisSecrets(secrets)
	if err != nil {
		return "", err
	}

	// TODO(allison): I'd really like ProvisionExtension to return the extension's ID, but for now we can just refetch it
	extFull, err := gql.GetAddOn(ctx, client.GenqClient(), bucketName, string(gql.AddOnTypeTigris))

	// Update the addon with the tokenized key and the name of the app
	_, err = gql.UpdateAddOn(ctx, client.GenqClient(), extFull.AddOn.Id, extFull.AddOn.AddOnPlan.Id, []string{}, extFull.AddOn.Options, map[string]interface{}{
		staticsMetaKeyAppId:      internalAppIdStr,
		staticsMetaTokenizedAuth: tokenizedKey,
	})
	if err != nil {
		return "", err
	}
	return tokenizedKey, nil
}

func (deployer *DeployerState) tokenizeTigrisSecrets(secrets map[string]interface{}) (string, error) {

	orgId, err := strconv.ParseUint(deployer.org.InternalNumericID, 10, 64)
	if err != nil {
		return "", fmt.Errorf("failed to decode org ID for %s: %w", deployer.org.Slug, err)
	}

	// TODO(allison): How do we handle moving an app between orgs?
	//                We're locking this token behind a hard dependency on the App ID and Org ID, but the Org ID
	//                will change when moving from one org to another.
	secret := &tokenizer.Secret{
		AuthConfig: &tokenizer.FlyioMacaroonAuthConfig{Access: flyio.Access{
			Action: resset.ActionWrite,
			OrgID:  &orgId,
			AppID:  fly.Pointer(uint64(deployer.app.InternalNumericID)),
		}},
		ProcessorConfig: &tokenizer.Sigv4ProcessorConfig{
			AccessKey: secrets["AWS_ACCESS_KEY_ID"].(string),
			SecretKey: secrets["AWS_SECRET_ACCESS_KEY"].(string),
		},
		RequestValidators: []tokenizer.RequestValidator{tokenizer.AllowHosts(fmt.Sprintf("%s.%s", deployer.bucket, tigrisHostname))},
	}

	return secret.Seal(tokenizerSealKey)
}
