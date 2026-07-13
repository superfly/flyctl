package volumes

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command/costestimate"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func TestRunVolumeEstimateBuildsRequest(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), ios)
	ctx = flyutil.NewContextWithClient(ctx, &mock.Client{
		GetAppCompactFunc: func(ctx context.Context, appName string) (*fly.AppCompact, error) {
			require.Equal(t, "test-app", appName)
			return &fly.AppCompact{Organization: &fly.OrganizationBasic{Slug: "test-org"}}, nil
		},
	})

	var gotOrgSlug string
	var gotReq uiex.CostEstimateRequest
	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
		CreateCostEstimateFunc: func(ctx context.Context, orgSlug string, in uiex.CostEstimateRequest) (*uiex.CostEstimateResponse, error) {
			gotOrgSlug = orgSlug
			gotReq = in

			return &uiex.CostEstimateResponse{Data: json.RawMessage(`{"ok":true}`)}, nil
		},
	})

	err := runVolumeEstimate(ctx, "test-app", volumeEstimateInput{
		Operation:     "volume.extend",
		SourceCommand: "fly volumes extend",
		Changes: []uiex.CostEstimateChange{
			{
				Kind:    "volume",
				Action:  "extend",
				Ref:     "vol_123",
				Count:   1,
				Current: volumeEstimateSpec{Region: "iad", SizeGB: 10},
				Desired: volumeEstimateSpec{Region: "iad", SizeGB: 20},
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, "test-org", gotOrgSlug)
	require.Equal(t, "volume.extend", gotReq.Operation)
	require.Equal(t, "fly volumes extend", gotReq.Client.SourceCommand)
	require.Len(t, gotReq.Changes, 1)
	require.Equal(t, "volume", gotReq.Changes[0].Kind)
	require.Equal(t, "extend", gotReq.Changes[0].Action)
	require.Equal(t, volumeEstimateSpec{Region: "iad", SizeGB: 10}, gotReq.Changes[0].Current)
	require.Equal(t, volumeEstimateSpec{Region: "iad", SizeGB: 20}, gotReq.Changes[0].Desired)
	require.JSONEq(t, `{"ok":true}`, out.String())
}

func TestVolumeEstimateOrgSlugResolvesPersonalRawSlug(t *testing.T) {
	ctx := flyutil.NewContextWithClient(context.Background(), &mock.Client{
		GetOrganizationBySlugFunc: func(ctx context.Context, slug string) (*fly.Organization, error) {
			require.Equal(t, "personal", slug)
			return &fly.Organization{RawSlug: "personal-raw"}, nil
		},
	})

	slug, err := costestimate.ResolveOrgSlug(ctx, &fly.OrganizationBasic{Slug: "personal"})

	require.NoError(t, err)
	require.Equal(t, "personal-raw", slug)
}
