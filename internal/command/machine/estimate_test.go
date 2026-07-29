package machine

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func TestRunMachineChangeEstimateBuildsUpdateRequest(t *testing.T) {
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

	app := &fly.AppCompact{
		Organization: &fly.OrganizationBasic{Slug: "test-org"},
	}
	current := &fly.Machine{
		ID:     "machine-id",
		Region: "iad",
		Config: &fly.MachineConfig{Guest: &fly.MachineGuest{CPUKind: "shared", CPUs: 1, MemoryMB: 256}},
	}
	desired := fly.LaunchMachineInput{
		Name:   "machine-name",
		Region: "iad",
		Config: &fly.MachineConfig{Guest: &fly.MachineGuest{CPUKind: "performance", CPUs: 2, MemoryMB: 4096}},
	}

	err := runMachineChangeEstimate(ctx, app, machineEstimateInput{
		Operation:      "machine.update",
		SourceCommand:  "fly machine update",
		Action:         "update",
		Current:        current,
		Desired:        desired,
		RunningSeconds: 3600,
	})

	require.NoError(t, err)
	require.Equal(t, "test-org", gotOrgSlug)
	require.Equal(t, "machine.update", gotReq.Operation)
	require.Equal(t, "fly machine update", gotReq.Client.SourceCommand)
	require.Len(t, gotReq.Changes, 1)
	require.Equal(t, "machine", gotReq.Changes[0].Kind)
	require.Equal(t, "update", gotReq.Changes[0].Action)
	require.Equal(t, current, gotReq.Changes[0].Current)
	require.Equal(t, desired, gotReq.Changes[0].Desired)
	require.Equal(t, 3600, gotReq.Changes[0].Usage["running_seconds"])
	require.JSONEq(t, `{"ok":true}`, out.String())
}
