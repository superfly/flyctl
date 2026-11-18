package redis

import (
	"context"
	"testing"

	genq "github.com/Khan/genqlient/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

// runCreateWithoutTTY runs `fly redis create <args>` with no terminal against mock APIs.
func runCreateWithoutTTY(t *testing.T, api *recordingAPI, args ...string) (stdout string, err error) {
	t.Helper()

	io, _, out, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)
	ctx = config.NewContext(ctx, &config.Config{})
	ctx = flyutil.NewContextWithClient(ctx, &mock.Client{
		GenqClientFunc: func() genq.Client { return api.client() },
	})
	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
		ListOrganizationsFunc: func(context.Context, bool) ([]uiex.Organization, error) {
			return []uiex.Organization{{ID: "org_1", Slug: "personal", Personal: true}}, nil
		},
	})
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetRegionsFunc: func(context.Context) (*flaps.RegionData, error) {
			return &flaps.RegionData{Regions: []fly.Region{{Code: "iad"}, {Code: "lhr"}}}, nil
		},
	})
	flags := newCreate().Flags()
	require.NoError(t, flags.Parse(args))
	ctx = flagctx.NewContext(ctx, flags)

	err = runCreate(ctx)

	return out.String(), err
}

func createAPI() *recordingAPI {
	return &recordingAPI{replies: map[string]string{
		"GetAddOnProvider": `{"addOnProvider": {"excludedRegions": []}}`,
		"ListAddOnPlans":   `{"addOnPlans": {"nodes": [{"id": "plan_fixed", "displayName": "Fixed 250MB"}]}}`,
		"CreateAddOn":      `{"createAddOn": {"addOn": {"name": "generated-name", "publicUrl": "redis://default:pw@fly-generated-name.upstash.io:6379"}}}`,
	}}
}

// ProdPack and replicas are billed, so with no TTY they must stay off.
func TestCreateWithoutTTYUsesSafeDefaults(t *testing.T) {
	api := createAPI()

	stdout, err := runCreateWithoutTTY(t, api, "--region", "iad", "--plan", "Fixed 250MB")
	require.NoError(t, err)

	var input map[string]any
	for _, req := range api.requests {
		if req.op == "CreateAddOn" {
			input, _ = req.variables["input"].(map[string]any)
		}
	}
	require.NotNil(t, input, "CreateAddOn was not sent")
	assert.Equal(t, "", input["name"])
	assert.Equal(t, "plan_fixed", input["planId"])
	assert.Equal(t, "iad", input["primaryRegion"])
	assert.Empty(t, input["readRegions"])
	assert.Equal(t, map[string]any{}, input["options"])

	assert.Contains(t, stdout, "Eviction is off. Use --enable-eviction to turn it on.")
	assert.Contains(t, stdout, "Auto-upgrade is off. Use --enable-auto-upgrade to turn it on.")
	assert.Contains(t, stdout, "ProdPack is off. Use --enable-prodpack to turn it on.")
	assert.Contains(t, stdout, "No replica regions. Use --replica-regions to add them.")
}

// The primary region has no safe default, so with no TTY nothing is created.
func TestCreateWithoutTTYRequiresRegion(t *testing.T) {
	api := createAPI()

	_, err := runCreateWithoutTTY(t, api, "--name", "cache")
	require.ErrorContains(t, err, "region code must be specified when not running interactively")
	assert.True(t, prompt.IsNonInteractive(err))

	for _, req := range api.requests {
		assert.NotEqual(t, "CreateAddOn", req.op)
	}
}
