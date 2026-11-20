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
	const (
		paygTip  = "consider switching to a fixed-price plan"
		docsLink = "See https://docs.fly.io/upstash/redis#pricing"
	)

	tests := []struct {
		name        string
		plan        gql.ListAddOnPlansAddOnPlansAddOnPlanConnectionNodesAddOnPlan
		prodPack    bool
		readRegions []fly.Region
		want        string
		wantPayg    bool
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
			want: "billed at $10/mo + $5 per read region (Fixed 250MB). " + docsLink,
		},
		{
			name: "fixed plan without description",
			plan: gql.ListAddOnPlansAddOnPlansAddOnPlanConnectionNodesAddOnPlan{Id: "fixed_250", DisplayName: "Fixed 250MB", PricePerMonth: 1000},
			want: "billed at $10/mo (Fixed 250MB). " + docsLink,
		},
		{
			name:     "pay-as-you-go with ProdPack",
			plan:     gql.ListAddOnPlansAddOnPlansAddOnPlanConnectionNodesAddOnPlan{Id: redisPlanPayAsYouGo, DisplayName: "Pay-as-you-go", Description: "$0.2 per 100K commands"},
			prodPack: true,
			want:     "billed at $0.20 per 100K commands, plus $200/mo for ProdPack.",
			wantPayg: true,
		},
		{
			name:     "fixed plan with ProdPack",
			plan:     gql.ListAddOnPlansAddOnPlansAddOnPlanConnectionNodesAddOnPlan{Id: "fixed_250", DisplayName: "Fixed 250MB", Description: "$10/mo + $5 per read region", PricePerMonth: 1000},
			prodPack: true,
			want:     "billed at $10/mo + $5 per read region (Fixed 250MB), plus $200/mo for ProdPack. " + docsLink,
		},
		{
			name:        "fixed plan with read regions",
			plan:        gql.ListAddOnPlansAddOnPlansAddOnPlanConnectionNodesAddOnPlan{Id: "fixed_250", DisplayName: "Fixed 250MB", Description: "$10/mo + $5 per read region", PricePerMonth: 1000},
			readRegions: []fly.Region{{Code: "iad"}, {Code: "lhr"}},
			want:        "billed at $10/mo + $5 per read region (Fixed 250MB), with 2 read regions. " + docsLink,
		},
		{
			name:        "fixed plan with one read region and ProdPack",
			plan:        gql.ListAddOnPlansAddOnPlansAddOnPlanConnectionNodesAddOnPlan{Id: "fixed_250", DisplayName: "Fixed 250MB", Description: "$10/mo + $5 per read region", PricePerMonth: 1000},
			prodPack:    true,
			readRegions: []fly.Region{{Code: "iad"}},
			want:        "billed at $10/mo + $5 per read region (Fixed 250MB), with 1 read region, plus $200/mo for ProdPack. " + docsLink,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &recordingAPI{replies: map[string]string{
				"GetAddOnProvider": `{"addOnProvider": {"excludedRegions": []}}`,
				"CreateAddOn":      `{"createAddOn": {"addOn": {"name": "cache", "publicUrl": "redis://cache"}}}`,
			}}
			create := func(ctx context.Context) error {
				readRegions := append([]fly.Region{}, tt.readRegions...)
				_, err := Create(ctx, &uiex.Organization{Slug: "personal"}, "cache", &fly.Region{Code: "ord"}, &tt.plan, true, false, false, tt.prodPack, &readRegions)

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
