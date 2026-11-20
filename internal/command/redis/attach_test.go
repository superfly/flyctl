package redis

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	graphql "github.com/Khan/genqlient/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

// chdir changes the working directory for the duration of the test.
// appsecrets.Update (via internal/config's lock file) resolves its lock
// path relative to flyctl's package-global config dir, which is unset in
// tests and otherwise resolves relative to the current working directory.
func chdir(tb testing.TB, dir string) {
	tb.Helper()

	prev, err := os.Getwd()
	if err != nil {
		tb.Fatalf("cannot read working directory: %s", err)
	}
	if err := os.Chdir(dir); err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() {
		tb.Helper()
		if err := os.Chdir(prev); err != nil {
			tb.Fatalf("cannot revert working directory: %s", err)
		}
	})
}

type attachGenqClient struct {
	t         *testing.T
	publicUrl string
}

func (c *attachGenqClient) MakeRequest(_ context.Context, req *graphql.Request, resp *graphql.Response) error {
	if req.OpName != "GetAddOn" {
		c.t.Fatalf("unexpected GraphQL operation %q", req.OpName)
	}

	data, err := json.Marshal(map[string]any{
		"addOn": map[string]any{
			"id":        "addon-id",
			"name":      "test-redis",
			"status":    "ready",
			"publicUrl": c.publicUrl,
			"organization": map[string]any{
				"slug": "test-org",
			},
			"addOnProvider": map[string]any{
				"name": "upstash_redis",
			},
		},
	})
	require.NoError(c.t, err)

	return json.Unmarshal(data, resp.Data)
}

func TestRunAttachSetsRedisUrlSecret(t *testing.T) {
	chdir(t, t.TempDir())

	genqClient := &attachGenqClient{t: t, publicUrl: "redis://default:secret@fly-test-redis.upstash.io"}
	var updatedApp string
	var updatedSecrets map[string]*string

	flapsClient := &mock.FlapsClient{
		UpdateAppSecretsFunc: func(_ context.Context, appName string, values map[string]*string) (*fly.UpdateAppSecretsResp, error) {
			updatedApp = appName
			updatedSecrets = values

			return &fly.UpdateAppSecretsResp{}, nil
		},
	}

	client := &mock.Client{GenqClientFunc: func() graphql.Client { return genqClient }}

	cmd := newAttach()
	require.NoError(t, cmd.Flags().Parse([]string{"--app", "test-app", "test-redis"}))

	io, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)
	ctx = flag.NewContext(ctx, cmd.Flags())
	ctx = flyutil.NewContextWithClient(ctx, client)
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)
	ctx = appconfig.WithName(ctx, "test-app")
	ctx = state.WithConfigDirectory(ctx, t.TempDir())

	require.NoError(t, runAttach(ctx))

	assert.Equal(t, "test-app", updatedApp)
	require.Contains(t, updatedSecrets, "REDIS_URL")
	require.NotNil(t, updatedSecrets["REDIS_URL"])
	assert.Equal(t, "redis://default:secret@fly-test-redis.upstash.io", *updatedSecrets["REDIS_URL"])
}

func TestRunAttachRejectsEmptyPublicUrl(t *testing.T) {
	chdir(t, t.TempDir())

	genqClient := &attachGenqClient{t: t, publicUrl: ""}
	updateCalled := false

	flapsClient := &mock.FlapsClient{
		UpdateAppSecretsFunc: func(_ context.Context, appName string, values map[string]*string) (*fly.UpdateAppSecretsResp, error) {
			updateCalled = true

			return &fly.UpdateAppSecretsResp{}, nil
		},
	}

	client := &mock.Client{GenqClientFunc: func() graphql.Client { return genqClient }}

	cmd := newAttach()
	require.NoError(t, cmd.Flags().Parse([]string{"--app", "test-app", "test-redis"}))

	io, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)
	ctx = flag.NewContext(ctx, cmd.Flags())
	ctx = flyutil.NewContextWithClient(ctx, client)
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)
	ctx = appconfig.WithName(ctx, "test-app")
	ctx = state.WithConfigDirectory(ctx, t.TempDir())

	require.Error(t, runAttach(ctx))
	assert.False(t, updateCalled)
}
