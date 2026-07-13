package ips

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func TestRunIPEstimateBuildsRequest(t *testing.T) {
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

	err := runIPEstimate(ctx, "test-app", "ip.allocate-egress", "fly ips allocate-egress", ipEstimateSpec{Family: "v4", Type: "egress", Region: "iad"})

	require.NoError(t, err)
	require.Equal(t, "test-org", gotOrgSlug)
	require.Equal(t, "ip.allocate-egress", gotReq.Operation)
	require.Equal(t, "fly ips allocate-egress", gotReq.Client.SourceCommand)
	require.Len(t, gotReq.Changes, 1)
	require.Equal(t, "ip", gotReq.Changes[0].Kind)
	require.Equal(t, "allocate", gotReq.Changes[0].Action)
	require.Equal(t, "egress", gotReq.Changes[0].Ref)
	require.Equal(t, ipEstimateSpec{Family: "v4", Type: "egress", Region: "iad"}, gotReq.Changes[0].Desired)
	require.JSONEq(t, `{"ok":true}`, out.String())
}
