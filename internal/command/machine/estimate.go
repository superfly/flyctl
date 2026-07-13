package machine

import (
	"context"
	"fmt"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command/costestimate"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/uiex"
)

// runMachineEstimate prices the Machine that `machine run`/`machine create`
// would create and prints a JSON cost estimate. It runs after flyctl resolves
// the app and Machine config, but before any Machine API writes.
func runMachineEstimate(ctx context.Context, app *fly.AppCompact, input fly.LaunchMachineInput, isCreate bool) error {
	operation := "machine.run"
	sourceCommand := "fly machine run"
	runningSeconds := 3600
	action := "create"
	if isCreate {
		operation = "machine.create"
		sourceCommand = "fly machine create"
		runningSeconds = 0
	}

	return runMachineChangeEstimate(ctx, app, machineEstimateInput{
		Operation:      operation,
		SourceCommand:  sourceCommand,
		Action:         action,
		Desired:        input,
		RunningSeconds: runningSeconds,
	})
}

type machineEstimateInput struct {
	Operation      string
	SourceCommand  string
	Action         string
	Current        any
	Desired        any
	RunningSeconds int
}

func runMachineChangeEstimate(ctx context.Context, app *fly.AppCompact, input machineEstimateInput) error {
	usage := map[string]any{}
	if input.RunningSeconds > 0 {
		usage["running_seconds"] = input.RunningSeconds
	}

	return costestimate.RunForOrg(ctx, app.Organization, costestimate.Input{
		Operation: input.Operation,
		Changes: []uiex.CostEstimateChange{
			{
				Kind:    "machine",
				Action:  input.Action,
				Ref:     "machine",
				Count:   1,
				Current: input.Current,
				Desired: input.Desired,
				Usage:   usage,
			},
		},
		SourceCommand: input.SourceCommand,
	})
}

func estimateApp(ctx context.Context, appName string) (*fly.AppCompact, error) {
	client := flyutil.ClientFromContext(ctx)
	if client == nil {
		return nil, fmt.Errorf("can't estimate cost without an API client")
	}

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return nil, fmt.Errorf("failed fetching app: %w", err)
	}

	return app, nil
}
