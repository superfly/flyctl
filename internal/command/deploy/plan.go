package deploy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/go-cmp/cmp"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/ctrlc"
	"github.com/superfly/flyctl/internal/flapsutil"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/statuslogger"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

type AppState struct {
	Machines []*fly.Machine
}

type machinePairing struct {
	oldMachine *fly.Machine
	newMachine *fly.Machine
}

func (md *machineDeployment) appState(ctx context.Context) (*AppState, error) {
	machines, err := md.flapsClient.List(ctx, "")
	if err != nil {
		return nil, err
	}

	// TODO: could this be a list of machine id -> config?
	appState := &AppState{
		Machines: machines,
	}

	return appState, nil
}

type healthcheckResult struct {
	regularChecksPassed bool
	smokeChecksPassed   bool
	machineChecksPassed bool
}

var healthChecksPassed = sync.Map{}

func (md *machineDeployment) updateMachines(ctx context.Context, oldAppState, newAppState *AppState, pushForward bool, statusLogger statuslogger.StatusLogger, skipHealthChecks bool, skipSmokeChecks bool) error {
	ctx, cancel := context.WithCancel(ctx)
	ctx, cancel = ctrlc.HookCancelableContext(ctx, cancel)
	defer cancel()

	oldMachines := make(map[string]*fly.Machine)
	for _, machine := range oldAppState.Machines {
		oldMachines[machine.ID] = machine
	}
	newMachines := make(map[string]*fly.Machine)
	for _, machine := range newAppState.Machines {
		newMachines[machine.ID] = machine
	}

	machineTuples := make([]machinePairing, 0)
	for _, oldMachine := range oldMachines {
		// This means we want to update a machine
		if newMachine, ok := newMachines[oldMachine.ID]; ok {
			healthChecksPassed.LoadOrStore(oldMachine.ID, &healthcheckResult{
				regularChecksPassed: skipHealthChecks,
				machineChecksPassed: skipHealthChecks,
				smokeChecksPassed:   skipSmokeChecks,
			})
			machineTuples = append(machineTuples, machinePairing{oldMachine: oldMachine, newMachine: newMachine})
		}
	}

	for _, newMachine := range newMachines {
		if _, ok := oldMachines[newMachine.ID]; !ok {
			// This means we should create the new machine
			healthChecksPassed.LoadOrStore(newMachine.ID, &healthcheckResult{
				regularChecksPassed: skipHealthChecks,
				machineChecksPassed: skipHealthChecks,
				smokeChecksPassed:   skipSmokeChecks,
			})
			machineTuples = append(machineTuples, machinePairing{oldMachine: nil, newMachine: newMachine})
		}
	}

	var sl statuslogger.StatusLogger
	if statusLogger != nil {
		sl = statusLogger
	} else {
		sl = statuslogger.Create(ctx, len(machineTuples), true)
		defer sl.Destroy(false)
	}

	group := errgroup.Group{}
	group.SetLimit(md.maxConcurrent)
	for idx, machPair := range machineTuples {
		machPair := machPair
		oldMachine := machPair.oldMachine
		newMachine := machPair.newMachine

		idx := idx
		group.Go(func() error {
			checkResult, _ := healthChecksPassed.Load(machPair.oldMachine.ID)
			machineCheckResult := checkResult.(*healthcheckResult)
			err := md.updateMachineWChecks(ctx, oldMachine, newMachine, idx, sl, md.io, machineCheckResult)
			if err != nil {
				sl.Line(idx).LogStatus(statuslogger.StatusFailure, err.Error())
				return fmt.Errorf("failed to update machine %s: %w", oldMachine.ID, err)
			}
			return nil
		})
	}

	if updateErr := group.Wait(); updateErr != nil {
		if !pushForward {
			return updateErr
		}

		var unrecoverableErr *unrecoverableError
		if errors.As(updateErr, &unrecoverableErr) || errors.Is(updateErr, context.Canceled) {
			return updateErr
		}

		attempts := 0
		// if we fail to update the machines, we should revert the state back if possible
		for {
			if attempts > md.deployRetries {
				return updateErr
			}

			currentState, err := md.appState(ctx)
			if err != nil {
				fmt.Println("Failed to get current state:", err)
				return err
			}
			err = md.updateMachines(ctx, currentState, newAppState, false, sl, skipHealthChecks, skipSmokeChecks)
			if err == nil {
				break
			} else if errors.Is(err, context.Canceled) {
				return err
			} else {
				if errors.As(err, &unrecoverableErr) || errors.Is(err, context.Canceled) {
					return err
				}
				fmt.Println("Failed to update machines:", err, ". Retrying...")
			}
			attempts += 1
			time.Sleep(1 * time.Second)
		}

		return nil
	}

	return nil
}

type unrecoverableError struct {
	err error
}

func (e *unrecoverableError) Error() string {
	return fmt.Sprintf("Unrecoverable error: %s", e.err)
}

func (e *unrecoverableError) Unwrap() error {
	return e.err
}

func compareConfigs(oldConfig, newConfig *fly.MachineConfig) bool {
	opt := cmp.FilterPath(func(p cmp.Path) bool {
		vx := p.Last().String()

		if vx == `["fly_flyctl_version"]` {
			return true
		}
		return false
	}, cmp.Ignore())

	return cmp.Equal(oldConfig, newConfig, opt)
}

func (md *machineDeployment) updateMachineWChecks(ctx context.Context, oldMachine, newMachine *fly.Machine, idx int, sl statuslogger.StatusLogger, io *iostreams.IOStreams, healthcheckResult *healthcheckResult) error {
	var machine *fly.Machine = oldMachine
	var lease *fly.MachineLease

	defer func() {
		if machine == nil || lease == nil {
			return
		}

		// even if we fail to update the machine, we need to clear the lease
		ctx := context.WithoutCancel(ctx)
		err := clearMachineLease(ctx, machine.ID, lease.Data.Nonce)
		if err != nil {
			fmt.Println("Failed to clear lease for machine", machine.ID, "due to error", err)
			sl.Line(idx).LogStatus(statuslogger.StatusFailure, fmt.Sprintf("Failed to clear lease for machine %s", machine.ID))
		}
	}()

	var err error

	machine, lease, err = md.updateOrCreateMachine(ctx, oldMachine, newMachine, sl.Line(idx))
	if err != nil || (machine == nil && lease == nil) {
		return err
	}

	flapsClient := flapsutil.ClientFromContext(ctx)
	lm := mach.NewLeasableMachine(flapsClient, io, machine, false)

	shouldStart := newMachine.State == "started" || newMachine.State == "replacing"

	if !shouldStart {
		sl.Line(idx).LogStatus(statuslogger.StatusSuccess, fmt.Sprintf("Machine %s is now in a good state", machine.ID))
		return nil
	}

	if !healthcheckResult.machineChecksPassed || !healthcheckResult.smokeChecksPassed {
		sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Waiting for machine %s to reach a good state", oldMachine.ID))
		state, err := waitForMachineState(ctx, lm, []string{"stopped", "started", "suspended"}, md.waitTimeout, sl.Line(idx))
		if err != nil {
			return err
		}

		if state != "started" {
			sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Starting machine %s", oldMachine.ID))
			err = startMachine(ctx, machine.ID, lease.Data.Nonce)
			if err != nil {
				return err
			}

			sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Waiting for machine %s to reach the 'started' state", machine.ID))
			_, err = waitForMachineState(ctx, lm, []string{"started", "stopped"}, md.waitTimeout, sl.Line(idx))
			if err != nil {
				return err
			}
		}

	}

	if !healthcheckResult.smokeChecksPassed {
		sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Running smoke checks on machine %s", machine.ID))
		err = md.doSmokeChecks(ctx, lm, false)
		if err != nil {
			return &unrecoverableError{err: err}
		}
		healthcheckResult.smokeChecksPassed = true
	}

	if !healthcheckResult.machineChecksPassed {
		sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Running machine checks on machine %s", machine.ID))
		err = md.runTestMachines(ctx, machine, sl.Line(idx))
		if err != nil {
			return &unrecoverableError{err: err}
		}
		healthcheckResult.machineChecksPassed = true
	}

	if !healthcheckResult.regularChecksPassed {
		sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Checking health of machine %s", machine.ID))
		err = lm.WaitForHealthchecksToPass(ctx, md.waitTimeout)
		if err != nil {
			return &unrecoverableError{err: err}
		}
		healthcheckResult.regularChecksPassed = true
	}

	sl.Line(idx).LogStatus(statuslogger.StatusSuccess, fmt.Sprintf("Machine %s is now in a good state", machine.ID))

	return nil
}

func (md *machineDeployment) updateOrCreateMachine(ctx context.Context, oldMachine, newMachine *fly.Machine, sl statuslogger.StatusLine) (*fly.Machine, *fly.MachineLease, error) {
	if oldMachine != nil {
		sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Acquiring lease for %s", oldMachine.ID))
		lease, err := acquireMachineLease(ctx, oldMachine.ID)
		if err != nil {
			return nil, nil, err
		}

		if newMachine == nil {
			return nil, nil, destroyMachine(ctx, oldMachine.ID, lease.Data.Nonce)
		} else {
			oldMachine.LeaseNonce = lease.Data.Nonce
			sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Updating machine config for %s", oldMachine.ID))
			machine, err := md.updateMachineConfig(ctx, oldMachine, newMachine.Config, sl, newMachine.State == "replacing")
			if err != nil {
				return oldMachine, lease, err
			}

			return machine, lease, nil
		}
	} else if newMachine != nil {
		sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Creating machine for %s", newMachine.ID))
		machine, err := createMachine(ctx, newMachine.Config, newMachine.Region)
		if err != nil {
			return nil, nil, err
		}

		sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Acquiring lease for %s", newMachine.ID))
		lease, err := acquireMachineLease(ctx, machine.ID)
		if err != nil {
			return nil, nil, err
		}

		return machine, lease, nil
	} else {
		// both old and new machines are nil, so just a noop
		return nil, nil, nil
	}
}

func destroyMachine(ctx context.Context, machineID string, lease string) error {
	flapsClient := flapsutil.ClientFromContext(ctx)
	err := flapsClient.Destroy(ctx, fly.RemoveMachineInput{
		ID:   machineID,
		Kill: true,
	}, lease)
	if err != nil {
		return err
	}

	return nil
}

func clearMachineLease(ctx context.Context, machID, leaseNonce string) error {
	// TODO: remove this when valentin's work is done
	flapsClient := flapsutil.ClientFromContext(ctx)
	attempts := 0
	for {
		err := flapsClient.ReleaseLease(ctx, machID, leaseNonce)
		if err == nil {
			return nil
		}
		attempts += 1
		if attempts > 5 {
			return err
		}
		time.Sleep(1 * time.Second)
	}
}

// returns when the machine is in one of the possible states, or after passing the timeout threshold
func waitForMachineState(ctx context.Context, lm mach.LeasableMachine, possibleStates []string, timeout time.Duration, sl statuslogger.StatusLine) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var mutex sync.Mutex

	var waitErr error
	numCompleted := 0
	var successfulState string

	for _, state := range possibleStates {
		state := state
		go func() {
			err := lm.WaitForState(ctx, state, timeout, false)
			mutex.Lock()
			defer func() {
				numCompleted += 1
				mutex.Unlock()
			}()

			if successfulState != "" {
				return
			}
			sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Machine %s reached %s state", lm.Machine().ID, state))

			if err != nil {
				waitErr = err
			} else {
				successfulState = state
			}
		}()
	}

	// TODO(billy): i'm sure we can use channels here
	for {
		mutex.Lock()
		if successfulState != "" || numCompleted == len(possibleStates) {
			defer mutex.Unlock()
			return successfulState, waitErr
		}
		mutex.Unlock()

		time.Sleep(1 * time.Second)
	}
}

func acquireMachineLease(ctx context.Context, machID string) (*fly.MachineLease, error) {
	flapsClient := flapsutil.ClientFromContext(ctx)
	lease, err := flapsClient.AcquireLease(ctx, machID, fly.IntPointer(3600))
	if err != nil {
		// TODO: tell users how to manually clear the lease
		// TODO: have a flag to automatically clear the lease
		if strings.Contains(err.Error(), "failed to get lease") {
			return nil, &unrecoverableError{err: err}
		} else {
			return nil, err
		}
	}

	return lease, nil
}

func (md *machineDeployment) updateMachineConfig(ctx context.Context, oldMachine *fly.Machine, newMachineConfig *fly.MachineConfig, sl statuslogger.StatusLine, shouldReplace bool) (*fly.Machine, error) {
	if compareConfigs(oldMachine.Config, newMachineConfig) {
		return oldMachine, nil
	}

	input, err := md.launchInputForUpdate(oldMachine)
	if err != nil {
		return nil, err
	}
	input.Config = newMachineConfig
	if shouldReplace {
		input.RequiresReplacement = shouldReplace
	}

	lm := mach.NewLeasableMachine(md.flapsClient, md.io, oldMachine, false)
	entry := &machineUpdateEntry{
		leasableMachine: lm,
		launchInput:     input,
	}
	err = md.updateMachine(ctx, entry, sl)
	if err != nil {
		if strings.Contains(err.Error(), "deploys to this host are temporarily disabled") {
			err := md.updateMachine(ctx, entry, sl)

			if err != nil {
				return nil, err
			}
		}

		return nil, err
	}
	return lm.Machine(), nil
}

func createMachine(ctx context.Context, machConfig *fly.MachineConfig, region string) (*fly.Machine, error) {
	flapsClient := flapsutil.ClientFromContext(ctx)
	machine, err := flapsClient.Launch(ctx, fly.LaunchMachineInput{
		Config: machConfig,
		Region: region,
	})
	if err != nil {
		return nil, err
	}

	return machine, nil
}

func (md *machineDeployment) updateMachine(ctx context.Context, e *machineUpdateEntry, sl statuslogger.StatusLine) error {
	ctx, span := tracing.GetTracer().Start(ctx, "update_machine", trace.WithAttributes(
		attribute.String("id", e.launchInput.ID),
		attribute.Bool("requires_replacement", e.launchInput.RequiresReplacement),
	))
	defer span.End()

	fmtID := e.leasableMachine.FormattedMachineId()

	replaceMachine := func() error {
		sl.Logf("Replacing %s by new machine", fmtID)
		if err := md.updateMachineByReplace(ctx, e); err != nil {
			return err
		}
		sl.Logf("Created machine %s", fmtID)
		return nil
	}

	if e.launchInput.RequiresReplacement {
		return replaceMachine()
	}

	sl.Logf("Updating %s", md.colorize.Bold(fmtID))
	if err := md.updateMachineInPlace(ctx, e); err != nil {
		switch {
		case len(e.leasableMachine.Machine().Config.Mounts) > 0:
			// Replacing a machine with a volume will cause the placement logic to pick wthe same host
			// dismissing the value of replacing it in case of lack of host capacity
			return err
		case strings.Contains(err.Error(), "could not reserve resource for machine"),
			strings.Contains(err.Error(), "deploys to this host are temporarily disabled"):
			return replaceMachine()
		default:
			return err
		}
	}
	return nil
}

func startMachine(ctx context.Context, machineID string, leaseNonce string) error {
	flapsClient := flapsutil.ClientFromContext(ctx)
	_, err := flapsClient.Start(ctx, machineID, leaseNonce)
	if err != nil {
		if strings.Contains(err.Error(), "machine still active") {
			return nil
		}
		fmt.Println("Failed to start machine", machineID, "due to error", err)
		return err
	}

	return nil
}