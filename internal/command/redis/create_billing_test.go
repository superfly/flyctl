package redis

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/uiex"
)

func TestCreatePrintsPlanBilling(t *testing.T) {
	const paygTip = "consider switching to a fixed-price plan"

	tests := []struct {
		name     string
		plan     gql.ListAddOnPlansAddOnPlansAddOnPlanConnectionNodesAddOnPlan
		want     string
		wantPayg bool
	}{
		{
			name:     "pay-as-you-go",
			plan:     gql.ListAddOnPlansAddOnPlansAddOnPlanConnectionNodesAddOnPlan{Id: redisPlanPayAsYouGo, DisplayName: "Pay-as-you-go", Description: "$0.2 per 100K commands"},
			want:     "billed at $0.20 per 100K commands.",
			wantPayg: true,
		},
		{
			name: "fixed plan",
			plan: gql.ListAddOnPlansAddOnPlansAddOnPlanConnectionNodesAddOnPlan{Id: "fixed_250", DisplayName: "Fixed 250MB", Description: "$10/mo + $5 per read region", PricePerMonth: 1000},
			want: "billed at $10/mo + $5 per read region (Fixed 250MB).",
		},
		{
			name: "fixed plan without description",
			plan: gql.ListAddOnPlansAddOnPlansAddOnPlanConnectionNodesAddOnPlan{Id: "fixed_250", DisplayName: "Fixed 250MB", PricePerMonth: 1000},
			want: "billed at $10/mo (Fixed 250MB).",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &recordingAPI{replies: map[string]string{
				"GetAddOnProvider": `{"addOnProvider": {"excludedRegions": []}}`,
				"CreateAddOn":      `{"createAddOn": {"addOn": {"name": "cache", "publicUrl": "redis://cache"}}}`,
			}}
			create := func(ctx context.Context) error {
				_, err := Create(ctx, &uiex.Organization{Slug: "personal"}, "cache", &fly.Region{Code: "ord"}, &tt.plan, true, false, false, false, &[]fly.Region{})

				return err
			}

			stdout, _, err := runCommand(t, &cobra.Command{}, create, api)
			require.NoError(t, err)

			assert.Contains(t, stdout, tt.want)
			if tt.wantPayg {
				assert.Contains(t, stdout, paygTip)
			} else {
				assert.NotContains(t, stdout, paygTip)
			}
		})
	}
}
