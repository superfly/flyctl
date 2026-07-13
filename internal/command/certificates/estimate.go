package certificates

import (
	"context"

	"github.com/superfly/flyctl/internal/command/costestimate"
	"github.com/superfly/flyctl/internal/uiex"
)

type certificateEstimateSpec struct {
	Hostname string `json:"hostname"`
}

func runCertificateEstimate(ctx context.Context, appName string, operation string, sourceCommand string, action string, hostname string) error {
	change := uiex.CostEstimateChange{
		Kind:   "certificate",
		Action: action,
		Ref:    hostname,
		Count:  1,
	}
	spec := certificateEstimateSpec{Hostname: hostname}
	if action == "destroy" {
		change.Current = spec
	} else {
		change.Desired = spec
	}

	return costestimate.RunForApp(ctx, appName, costestimate.Input{
		Operation:     operation,
		Changes:       []uiex.CostEstimateChange{change},
		SourceCommand: sourceCommand,
	})
}
