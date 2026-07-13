package ips

import (
	"context"

	"github.com/superfly/flyctl/internal/command/costestimate"
	"github.com/superfly/flyctl/internal/uiex"
)

type ipEstimateSpec struct {
	Family string `json:"family"`
	Type   string `json:"type,omitempty"`
	Region string `json:"region,omitempty"`
}

func runIPEstimate(ctx context.Context, appName string, operation string, sourceCommand string, desired ipEstimateSpec) error {
	return costestimate.RunForApp(ctx, appName, costestimate.Input{
		Operation: operation,
		Changes: []uiex.CostEstimateChange{
			{
				Kind:    "ip",
				Action:  "allocate",
				Ref:     desired.Type,
				Count:   1,
				Desired: desired,
			},
		},
		SourceCommand: sourceCommand,
	})
}
