package estimate

import (
	"context"

	"github.com/superfly/flyctl/internal/command/costestimate"
	"github.com/superfly/flyctl/internal/uiex"
)

type CreateInput struct {
	Name           string `json:"name,omitempty"`
	Plan           string `json:"plan"`
	Region         string `json:"region"`
	StorageGB      int    `json:"storage_gb,omitempty"`
	PGMajorVersion int    `json:"pg_major_version,omitempty"`
	PostGISEnabled bool   `json:"postgis_enabled,omitempty"`
}

func RunCreate(ctx context.Context, orgSlug string, input CreateInput) error {
	return costestimate.RunForOrgSlug(ctx, orgSlug, costestimate.Input{
		Operation: "mpg.create",
		Changes: []uiex.CostEstimateChange{
			{
				Kind:    "mpg",
				Action:  "create",
				Ref:     "postgres",
				Count:   1,
				Desired: input,
			},
		},
		SourceCommand: "fly mpg create",
	})
}
