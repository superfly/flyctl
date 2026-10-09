package tigris

import (
	"context"
	"encoding/json"
	"testing"

	graphql "github.com/Khan/genqlient/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

type recordingGenqClient struct {
	t              *testing.T
	metadata       any
	options        map[string]any
	updateOptions  map[string]any
	updateMetadata any
}

func (c *recordingGenqClient) MakeRequest(_ context.Context, req *graphql.Request, resp *graphql.Response) error {
	switch req.OpName {
	case "GetAddOn":
		options := c.options
		if options == nil {
			options = map[string]any{"public": false}
		}

		decodeResponse(c.t, resp, map[string]any{
			"addOn": map[string]any{
				"id":       "addon-id",
				"name":     "test-bucket",
				"status":   "ready",
				"options":  options,
				"metadata": c.metadata,
				"addOnPlan": map[string]any{
					"id": "plan-id",
				},
				"addOnProvider": map[string]any{
					"name": "tigris",
				},
				"organization": map[string]any{
					"slug": "test-org",
				},
			},
		})
	case "UpdateAddOn":
		require.Equal(c.t, gql.UpdateAddOn_Operation, req.Query)

		variables := req.Variables.(interface {
			GetOptions() any
			GetMetadata() any
		})
		c.updateOptions = variables.GetOptions().(map[string]any)
		c.updateMetadata = variables.GetMetadata()

		decodeResponse(c.t, resp, map[string]any{
			"updateAddOn": map[string]any{
				"addOn": map[string]any{"id": "addon-id"},
			},
		})
	default:
		c.t.Fatalf("unexpected GraphQL operation %q", req.OpName)
	}

	return nil
}

func TestRunUpdatePreservesMetadataWithoutAliasingOptions(t *testing.T) {
	genqClient := runUpdateWithMetadata(t, map[string]any{
		"fly-statics-app-id":         "42",
		"fly-statics-bucket-name":    "test-statics",
		"fly-statics-tokenized-auth": "test-tokenized-auth",
		"provider": map[string]any{
			"regions": []any{"iad", "fra"},
		},
	})

	assert.Equal(t, map[string]any{"public": true}, genqClient.updateOptions)
	assert.Equal(t, map[string]any{
		"fly-statics-app-id":         "42",
		"fly-statics-bucket-name":    "test-statics",
		"fly-statics-tokenized-auth": "test-tokenized-auth",
		"provider": map[string]any{
			"regions": []any{"iad", "fra"},
		},
	}, genqClient.updateMetadata)
}

func TestRunUpdatePreservesNonObjectMetadata(t *testing.T) {
	genqClient := runUpdateWithMetadata(t, "opaque-provider-metadata")
	assert.Equal(t, "opaque-provider-metadata", genqClient.updateMetadata)
}

func TestRunUpdateNormalizesNilMetadata(t *testing.T) {
	genqClient := runUpdateWithMetadata(t, nil)
	require.IsType(t, map[string]any{}, genqClient.updateMetadata)
	assert.Empty(t, genqClient.updateMetadata)
}

func TestRunUpdatePreservesWriteThroughWhenFlagOmitted(t *testing.T) {
	genqClient := &recordingGenqClient{
		t: t,
		options: map[string]any{
			"shadow_bucket": map[string]any{
				"access_key":    "old",
				"secret_key":    "old",
				"region":        "us-east-1",
				"name":          "source-bucket",
				"endpoint":      "https://s3.us-east-1.amazonaws.com",
				"write_through": true,
			},
		},
	}
	client := &mock.Client{GenqClientFunc: func() graphql.Client { return genqClient }}

	cmd := update()
	require.NoError(t, cmd.Flags().Parse([]string{
		"--shadow-access-key", "123",
		"--shadow-secret-key", "abc",
		"--shadow-endpoint", "https://s3.us-east-1.amazonaws.com",
		"--shadow-region", "us-east-1",
		"--shadow-name", "source-bucket",
		"test-bucket",
	}))

	io, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)
	ctx = flag.NewContext(ctx, cmd.Flags())
	ctx = flyutil.NewContextWithClient(ctx, client)

	require.NoError(t, runUpdate(ctx))

	shadowBucket, ok := genqClient.updateOptions["shadow_bucket"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, shadowBucket["write_through"])
}

func TestRunUpdateSetsWriteThroughWhenFlagSpecified(t *testing.T) {
	genqClient := &recordingGenqClient{
		t: t,
		options: map[string]any{
			"shadow_bucket": map[string]any{
				"access_key":    "old",
				"secret_key":    "old",
				"region":        "us-east-1",
				"name":          "source-bucket",
				"endpoint":      "https://s3.us-east-1.amazonaws.com",
				"write_through": true,
			},
		},
	}
	client := &mock.Client{GenqClientFunc: func() graphql.Client { return genqClient }}

	cmd := update()
	require.NoError(t, cmd.Flags().Parse([]string{
		"--shadow-access-key", "123",
		"--shadow-secret-key", "abc",
		"--shadow-endpoint", "https://s3.us-east-1.amazonaws.com",
		"--shadow-region", "us-east-1",
		"--shadow-name", "source-bucket",
		"--shadow-write-through=false",
		"test-bucket",
	}))

	io, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)
	ctx = flag.NewContext(ctx, cmd.Flags())
	ctx = flyutil.NewContextWithClient(ctx, client)

	require.NoError(t, runUpdate(ctx))

	shadowBucket, ok := genqClient.updateOptions["shadow_bucket"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, shadowBucket["write_through"])
}

func runUpdateWithMetadata(t *testing.T, metadata any) *recordingGenqClient {
	t.Helper()

	genqClient := &recordingGenqClient{t: t, metadata: metadata}
	client := &mock.Client{GenqClientFunc: func() graphql.Client { return genqClient }}

	cmd := update()
	require.NoError(t, cmd.Flags().Parse([]string{"--public", "test-bucket"}))

	io, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)
	ctx = flag.NewContext(ctx, cmd.Flags())
	ctx = flyutil.NewContextWithClient(ctx, client)

	require.NoError(t, runUpdate(ctx))

	return genqClient
}

func decodeResponse(t *testing.T, resp *graphql.Response, payload any) {
	t.Helper()

	data, err := json.Marshal(payload)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, resp.Data))
}
