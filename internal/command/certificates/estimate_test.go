package certificates

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

func TestRunCertificateEstimateBuildsRequest(t *testing.T) {
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

	err := runCertificateEstimate(ctx, "test-app", "certs.add", "fly certs add", "create", "*.example.com")

	require.NoError(t, err)
	require.Equal(t, "test-org", gotOrgSlug)
	require.Equal(t, "certs.add", gotReq.Operation)
	require.Equal(t, "fly certs add", gotReq.Client.SourceCommand)
	require.Len(t, gotReq.Changes, 1)
	require.Equal(t, "certificate", gotReq.Changes[0].Kind)
	require.Equal(t, "create", gotReq.Changes[0].Action)
	require.Equal(t, "*.example.com", gotReq.Changes[0].Ref)
	require.Equal(t, certificateEstimateSpec{Hostname: "*.example.com"}, gotReq.Changes[0].Desired)
	require.JSONEq(t, `{"ok":true}`, out.String())
}
