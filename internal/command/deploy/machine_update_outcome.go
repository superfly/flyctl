package deploy

import (
	"context"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/machine"
)

func supportsPreservedStoppedUpdate(strategy, launchBasisState string) bool {
	if strategy != "canary" && strategy != "rolling" {
		return false
	}

	return launchBasisState == fly.MachineStateStarted || launchBasisState == "starting"
}

func isPreservedStoppedUpdate(current *fly.Machine, updatedInstanceID string) bool {
	if current == nil || current.Config == nil || current.Config.Schedule != "" ||
		current.State != fly.MachineStateStopped || current.InstanceID != updatedInstanceID {
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

func (md *machineDeployment) readPreservedStoppedUpdate(ctx context.Context, machineID, updatedInstanceID string) bool {
	current, err := md.flapsClient.Get(ctx, md.app.Name, machineID)

	return err == nil && isPreservedStoppedUpdate(current, updatedInstanceID)
}

func (md *machineDeployment) waitForStartedOrPreservedStoppedUpdate(
	ctx context.Context,
	lm machine.LeasableMachine,
	launchBasisState string,
	timeout time.Duration,
) (bool, error) {
	if !supportsPreservedStoppedUpdate(md.strategy, launchBasisState) {
		return false, lm.WaitForState(ctx, fly.MachineStateStarted, timeout, machine.WithJustCreated())
	}

	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	startedResult := make(chan error, 1)
	go func() {
		startedResult <- lm.WaitForState(waitCtx, fly.MachineStateStarted, timeout, machine.WithJustCreated())
	}()

	stoppedResult := make(chan error, 1)
	go func() {
		stoppedResult <- lm.WaitForState(waitCtx, fly.MachineStateStopped, timeout, machine.WithJustCreated())
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
			if err == nil && md.readPreservedStoppedUpdate(waitCtx, updated.ID, updated.InstanceID) {
				return true, nil
			}
			if startedResult == nil {
				return false, startedErr
			}
		case <-waitCtx.Done():
			return false, waitCtx.Err()
		}
	}
}
