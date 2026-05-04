package deploy

import (
	"context"
	"fmt"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/tracing"
)

func supportsPreservedStoppedUpdate(strategy, launchBasisState string) bool {
	if strategy != "canary" && strategy != "rolling" {
		return false
	}

	return launchBasisState == fly.MachineStateStarted || launchBasisState == "starting"
}

func isPreservedStoppedUpdate(current *fly.Machine, updatedVersion string) bool {
	if current == nil || current.Config == nil || current.Config.Schedule != "" ||
		current.State != fly.MachineStateStopped || current.Version != updatedVersion {
		return false
	}

	for _, event := range current.Events {
		if event == nil || event.Status == "" {
			continue
		}

		return event.Type == "update" && event.Status == fly.MachineStateStopped && event.Source == "flyd"
	}

	return false
}

func (md *machineDeployment) readPreservedStoppedUpdate(ctx context.Context, machineID, updatedVersion string) bool {
	current, err := md.flapsClient.Get(ctx, md.app.Name, machineID)

	return err == nil && isPreservedStoppedUpdate(current, updatedVersion)
}

func (md *machineDeployment) waitForStartedOrPreservedStoppedUpdate(
	ctx context.Context,
	lm machine.LeasableMachine,
	launchBasisState string,
	timeout time.Duration,
	previous *fly.Machine,
	opts ...machine.WaitOption,
) (preserved bool, err error) {
	ctx, span := tracing.GetTracer().Start(ctx, "wait_for_machine_state")
	defer func() {
		tracing.RecordError(ctx, span, err, "machine state wait failed")
		span.End()
	}()

	opts = append(opts, machine.WithJustCreated())
	if !supportsPreservedStoppedUpdate(md.strategy, launchBasisState) {
		return false, lm.WaitForState(ctx, fly.MachineStateStarted, timeout, opts...)
	}

	waitCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(fmt.Errorf("machine update outcome determined: %w", context.Canceled))

	startedResult := make(chan error, 1)
	go func() {
		startedResult <- lm.WaitForState(waitCtx, fly.MachineStateStarted, timeout, opts...)
	}()

	stoppedResult := make(chan error, 1)
	go func() {
		stoppedResult <- lm.WaitForState(waitCtx, fly.MachineStateStopped, timeout, opts...)
	}()

	updated := lm.Machine()
	var startedErr error
	for {
		select {
		case err := <-startedResult:
			startedResult = nil
			if err == nil {
				return false, nil
			}
			startedErr = err
			if stoppedResult == nil {
				return false, startedErr
			}
		case err := <-stoppedResult:
			stoppedResult = nil
			if err == nil {
				if previous == nil {
					if md.readPreservedStoppedUpdate(waitCtx, updated.ID, updated.Version) {
						return true, nil
					}
				} else {
					current, err := machine.VerifyUpdateApplied(waitCtx, md.flapsClient, md.app.Name, previous, updated, nil)
					if err != nil {
						return false, err
					}
					if isPreservedStoppedUpdate(current, updated.Version) {
						return true, nil
					}
				}
			}
			if startedResult == nil {
				return false, startedErr
			}
		case <-waitCtx.Done():
			return false, waitCtx.Err()
		}
	}
}
