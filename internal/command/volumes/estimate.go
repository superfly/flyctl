package volumes

import (
	"context"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command/costestimate"
	"github.com/superfly/flyctl/internal/uiex"
)

type volumeEstimateSpec struct {
	Region string `json:"region,omitempty"`
	SizeGB int    `json:"size_gb"`
}

type volumeEstimateInput struct {
	Operation     string
	SourceCommand string
	Changes       []uiex.CostEstimateChange
}

func runVolumeEstimate(ctx context.Context, appName string, input volumeEstimateInput) error {
	return costestimate.RunForApp(ctx, appName, costestimate.Input{
		Operation:     input.Operation,
		Changes:       input.Changes,
		SourceCommand: input.SourceCommand,
	})
}

func volumeSpec(vol *fly.Volume) volumeEstimateSpec {
	return volumeEstimateSpec{Region: vol.Region, SizeGB: vol.SizeGb}
}
