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

// Bucket is the subset of a tigris statics add-on that FindBucket's callers
// (destroy.go, move.go, ensureBucketCreated) need. It deliberately hides
// which GraphQL query produced the record so FindBucket can pick the most
// targeted path at runtime.
type Bucket struct {
	Name     string
	Metadata map[string]any
}

// bucketFromMetadata returns a Bucket if the given generated add-on node has
// the staticsMetaKeyAppId pointer matching app, else nil.
func bucketFromMetadata(name string, metadata interface{}, internalAppIdStr string) *Bucket {
	if metadata == nil {
		return nil
	}
	meta, ok := metadata.(map[string]any)
	if !ok {
		return nil
	}
	if meta[staticsMetaKeyAppId] != internalAppIdStr {
		return nil
	}

	return &Bucket{Name: name, Metadata: meta}
}

// FindBucket finds the tigris statics bucket for the given app and org.
// Returns nil, nil if no bucket is found.
//
// Lookup is tiered so common cases stay fast:
//  1. App-scoped: GetAppWithAddons resolves add-ons linked to the app via
//     add_ons.app_id. New buckets created by ensureBucketCreated are linked
//     this way and this is the only path they need.
//  2. Org-scoped fallback: ListOrganizationAddOns scans the org's tigris
//     add-ons and matches on the staticsMetaKeyAppId metadata pointer. This
//     catches legacy buckets created before app_id linking, which are not
//     being backfilled.
//
// Both paths verify the staticsMetaKeyAppId pointer — a user-provisioned
// tigris add-on that happens to be attached to the same app won't match.
func FindBucket(ctx context.Context, app *fly.App, org *fly.Organization) (*Bucket, error) {

	client := flyutil.ClientFromContext(ctx)
	gqlClient := client.GenqClient()

	// Using string comparison here because we might want to use BigInt app IDs in the future.
	internalAppIdStr := strconv.FormatUint(uint64(app.InternalNumericID), 10)

	appResp, err := gql.GetAppWithAddons(ctx, gqlClient, app.Name, gql.AddOnTypeTigris)
	if err != nil {
		return nil, err
	}
	for _, extension := range appResp.App.AddOns.Nodes {
		if bucket := bucketFromMetadata(extension.Name, extension.Metadata, internalAppIdStr); bucket != nil {
			return bucket, nil
		}
	}

	// Legacy fallback: old statics buckets weren't linked via add_ons.app_id,
	// so they only surface via the org-scoped query. Safe to drop once all
	// surviving buckets are known to be linked (or backfilled) by app_id.
	orgResp, err := gql.ListOrganizationAddOns(ctx, gqlClient, org.Slug, "tigris")
	if err != nil {
		return nil, err
	}
	for _, extension := range orgResp.Organization.AddOns.Nodes {
		if bucket := bucketFromMetadata(extension.Name, extension.Metadata, internalAppIdStr); bucket != nil {
			return bucket, nil
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
		deployer.bucket = bucket.Metadata[staticsMetaBucketName].(string)

		return bucket.Metadata[staticsMetaTokenizedAuth].(string), nil
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
		// AppName links the new add-on to the app via add_ons.app_id on the
		// server side. Without this, FindBucket has to fall back to matching
		// the staticsMetaKeyAppId metadata pointer, which forces a broader
		// (and slower) org-scoped search on every `fly apps destroy` and
		// `fly apps move`.
		AppName: deployer.appConfig.AppName,
	}
	params.Options["website"] = map[string]any{
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
			_, err := gql.DeleteAddOn(context.Background(), client, extName, string(gql.AddOnTypeTigris))
			if err != nil {
				fmt.Fprintf(iostreams.FromContext(ctx).ErrOut, "Failed to delete extension: %v\n", err)
			}
		}
	}()

	secrets := ext.Data.Environment.(map[string]any)

	deployer.bucket = secrets["BUCKET_NAME"].(string)

	tokenizedKey, err := deployer.tokenizeTigrisSecrets(secrets)
	if err != nil {
		return "", err
	}

	// TODO(allison): I'd really like ProvisionExtension to return the extension's ID, but for now we can just refetch it
	extFull, err := gql.GetAddOn(ctx, client.GenqClient(), extName, string(gql.AddOnTypeTigris))
	if err != nil {
		return "", err
	}

	// Update the addon with the tokenized key and the name of the app
	_, err = gql.UpdateAddOn(ctx, client.GenqClient(), extFull.AddOn.Id, extFull.AddOn.AddOnPlan.Id, []string{}, extFull.AddOn.Options, map[string]any{
		staticsMetaKeyAppId:      internalAppIdStr,
		staticsMetaTokenizedAuth: tokenizedKey,
		staticsMetaBucketName:    deployer.bucket,
	})
	if err != nil {
		return "", err
	}

	return tokenizedKey, nil
}

func (deployer *DeployerState) tokenizeTigrisSecrets(secrets map[string]any) (string, error) {

	orgId, err := strconv.ParseUint(deployer.org.InternalNumericID, 10, 64)
	if err != nil {
		return "", fmt.Errorf("failed to decode org ID for %s: %w", deployer.org.Slug, err)
	}

	secret := &tokenizer.Secret{
		AuthConfig: &tokenizer.FlyioMacaroonAuthConfig{Access: flyio.Access{
			Action: resset.ActionWrite,
			OrgID:  &orgId,
			AppID:  new(uint64(deployer.app.InternalNumericID)),
		}},
		ProcessorConfig: &tokenizer.Sigv4ProcessorConfig{
			AccessKey: secrets["AWS_ACCESS_KEY_ID"].(string),
			SecretKey: secrets["AWS_SECRET_ACCESS_KEY"].(string),
		},
		RequestValidators: []tokenizer.RequestValidator{tokenizer.AllowHosts(fmt.Sprintf("%s.%s", deployer.bucket, tigrisHostname))},
	}

	return secret.Seal(tokenizerSealKey)
}
