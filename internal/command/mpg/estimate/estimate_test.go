package estimate

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func TestRunCreateBuildsCostEstimateRequest(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), ios)

	var gotOrgSlug string
	var gotReq uiex.CostEstimateRequest
	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
		CreateCostEstimateFunc: func(ctx context.Context, orgSlug string, in uiex.CostEstimateRequest) (*uiex.CostEstimateResponse, error) {
			gotOrgSlug = orgSlug
			gotReq = in

			return &uiex.CostEstimateResponse{Data: json.RawMessage(`{"ok":true}`)}, nil
		},
	})

	err := RunCreate(ctx, "personal-raw", CreateInput{
		Name:           "my-db",
		Plan:           "starter",
		Region:         "iad",
		StorageGB:      10,
		PGMajorVersion: 17,
		PostGISEnabled: true,
	})

	require.NoError(t, err)
	require.Equal(t, "personal-raw", gotOrgSlug)
	require.Equal(t, 1, gotReq.SchemaVersion)
	require.Equal(t, "mpg.create", gotReq.Operation)
	require.Equal(t, "USD", gotReq.Currency)
	require.Len(t, gotReq.Changes, 1)
	require.Equal(t, "mpg", gotReq.Changes[0].Kind)
	require.Equal(t, "create", gotReq.Changes[0].Action)
	require.Equal(t, "postgres", gotReq.Changes[0].Ref)
	require.Equal(t, 1, gotReq.Changes[0].Count)
	require.Equal(t, "flyctl", gotReq.Client.Name)
	require.Equal(t, "fly mpg create", gotReq.Client.SourceCommand)
	require.JSONEq(t, `{"ok":true}`, out.String())

	desiredJSON, err := json.Marshal(gotReq.Changes[0].Desired)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"name":"my-db",
		"plan":"starter",
		"region":"iad",
		"storage_gb":10,
		"pg_major_version":17,
		"postgis_enabled":true
	}`, string(desiredJSON))
}
