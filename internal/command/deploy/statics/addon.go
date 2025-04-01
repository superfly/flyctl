package statics

import (
	"bytes"
	"context"
	"fmt"
	"io"
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
		meta := bucket.Metadata.(map[string]interface{})
		deployer.bucket = meta[staticsMetaBucketName].(string)
		return meta[staticsMetaTokenizedAuth].(string), nil
	}

	// Using string comparison here because we might want to use BigInt app IDs in the future.
	internalAppIdStr := strconv.FormatUint(uint64(deployer.app.InternalNumericID), 10)

	extName := fmt.Sprintf("%s-statics-%s", deployer.appConfig.AppName, haikunator.Haikunator().String())

	params := extensions.ExtensionParams{
		Organization:         deployer.org,
		Provider:             "tigris",
		Options:              gql.AddOnOptions{},
		ErrorCaptureCallback: nil,
		OverrideRegion:       deployer.appConfig.PrimaryRegion,
		OverrideName:         &extName,
	}
	params.Options["website"] = map[string]interface{}{
		"domain_name": "",
	}
	params.Options["accelerate"] = false
	// TODO(allison): Make sure we still need this when virtual services drop :)
	params.Options["public"] = true

	extCtx := iostreams.NewContext(ctx, &iostreams.IOStreams{
		In:     io.NopCloser(&bytes.Buffer{}),
		Out:    io.Discard,
		ErrOut: io.Discard,
	})
	ext, err := extensions.ProvisionExtension(extCtx, params)
	if err != nil {
		// If the extension name is taken, try again, haikunating the name.
		// If that fails too, return the original error. Otherwise, continue successfully
		if strings.Contains(err.Error(), "already exists for app") ||
			strings.Contains(err.Error(), "unavailable for creation") {
			extName = fmt.Sprintf("%s-%s", *params.OverrideName, haikunator.Haikunator().String())
			params.OverrideName = &extName
			newExt, newErr := extensions.ProvisionExtension(extCtx, params)
			if newErr == nil {
				ext = newExt
				err = nil
			}
		}
	}
	if err != nil {
		return "", err
	}

	defer func() {
		if retErr != nil {
			client := flyutil.ClientFromContext(ctx).GenqClient()
			// Using context.Background() here in case the error is that the context is canceled.
			_, err := gql.DeleteAddOn(context.Background(), client, extName)
			if err != nil {
				fmt.Fprintf(iostreams.FromContext(ctx).ErrOut, "Failed to delete extension: %v\n", err)
			}
		}
	}()

	secrets := ext.Data.Environment.(map[string]interface{})

	deployer.bucket = secrets["BUCKET_NAME"].(string)

	tokenizedKey, err := deployer.tokenizeTigrisSecrets(secrets)
	if err != nil {
		return "", err
	}

	// TODO(allison): I'd really like ProvisionExtension to return the extension's ID, but for now we can just refetch it
	extFull, err := gql.GetAddOn(ctx, client.GenqClient(), extName, string(gql.AddOnTypeTigris))

	// Update the addon with the tokenized key and the name of the app
	_, err = gql.UpdateAddOn(ctx, client.GenqClient(), extFull.AddOn.Id, extFull.AddOn.AddOnPlan.Id, []string{}, extFull.AddOn.Options, map[string]interface{}{
		staticsMetaKeyAppId:      internalAppIdStr,
		staticsMetaTokenizedAuth: tokenizedKey,
		staticsMetaBucketName:    deployer.bucket,
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
