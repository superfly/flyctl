package scale

import (
	"context"

	"github.com/superfly/flyctl/internal/command/costestimate"
	"github.com/superfly/flyctl/internal/uiex"
)

type scaleEstimateInput struct {
	Operation     string
	SourceCommand string
	Changes       []uiex.CostEstimateChange
}

type scaleVolumeSpec struct {
	Region string `json:"region,omitempty"`
	SizeGB int    `json:"size_gb"`
}

func runScaleEstimate(ctx context.Context, appName string, input scaleEstimateInput) error {
	return costestimate.RunForApp(ctx, appName, costestimate.Input{
		Operation:     input.Operation,
		Changes:       input.Changes,
		SourceCommand: input.SourceCommand,
	})
}
