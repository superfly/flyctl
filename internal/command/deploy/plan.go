package deploy

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/qmuntal/stateless"
	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/ctrlc"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/statuslogger"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

const (
	STOPPED_MACHINES_POOL_SIZE = 30
)

type MachineLogger struct {
	store map[string]statuslogger.StatusLine
	sl    statuslogger.StatusLogger
}

func NewMachineLogger(store map[string]statuslogger.StatusLine, sl statuslogger.StatusLogger) *MachineLogger {
	return &MachineLogger{
		store: store,
		sl:    sl,
	}
}

func (m *MachineLogger) initFromMachinePairs(mp []machinePairing) {
	for idx, machPair := range mp {
		if machPair.oldMachine != nil {
			m.store[machPair.oldMachine.ID] = m.sl.Line(idx)
		} else if machPair.newMachine != nil {
			m.store[machPair.newMachine.ID] = m.sl.Line(idx)
		}
	}
}

func (m *MachineLogger) getLoggerFromID(id string) statuslogger.StatusLine {
	return m.store[id]
}

type FSMState string

const (
	updateNotStarted FSMState = "updateNotStarted"

	updatingMachines       FSMState = "updatingMachines"
	failedUpdatingMachines FSMState = "failedUpdatingMachines"
	updatedMachines        FSMState = "updatedMachines"

	updateComplete FSMState = "updateComplete"
	updateFailed   FSMState = "updateFailed"
)

type fsmTrigger string

const (
	triggerUpdateMachines       fsmTrigger = "triggerUpdateMachines"
	triggerUpdateMachinesFailed fsmTrigger = "triggerUpdateMachinesFailed"
	triggerUpdateFailed         fsmTrigger = "triggerUpdateFailed"
	triggerUpdateComplete       fsmTrigger = "triggerUpdateComplete"
)

type AppState struct {
	Machines map[string]*fly.Machine
}

type machinePairing struct {
	oldMachine *fly.Machine
	newMachine *fly.Machine
}

func (md *machineDeployment) appState(ctx context.Context, existingAppState *AppState) (*AppState, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "app_state")
	defer span.End()

	machines, err := md.flapsClient.List(ctx, "")
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// we need to copy any leases we might have acquired
	if existingAppState != nil {
		for _, machine := range machines {
			if existingMachine, ok := existingAppState.Machines[machine.ID]; ok {
				machine.LeaseNonce = existingMachine.LeaseNonce
			}
		}
	}

	// TODO: could this be a list of machine id -> config?
	appState := &AppState{
		Machines: machineSliceToMap(machines),
	}

	return appState, nil
}

func machineSliceToMap(machs []*fly.Machine) map[string]*fly.Machine {
	return lo.KeyBy(machs, func(mach *fly.Machine) string {
		return mach.ID
	})
}

type healthcheckResult struct {
	regularChecksPassed bool
	smokeChecksPassed   bool
	machineChecksPassed bool
}

var healthChecksPassed = sync.Map{}

type updateMachineSettings struct {
	pushForward          bool
	skipHealthChecks     bool
	skipSmokeChecks      bool
	skipLeaseAcquisition bool
}

const rollingStrategyMaxConcurrentGroups = 16

func (md *machineDeployment) updateMachinesWRecovery(ctx context.Context, oldAppState, newAppState *AppState, statusLogger statuslogger.StatusLogger, settings updateMachineSettings) error {
	ctx, span := tracing.GetTracer().Start(
		ctx, "update_machines_w_recovery",
		trace.WithAttributes(attribute.Bool("push_forward", settings.pushForward)),
		trace.WithAttributes(attribute.Bool("skip_health_checks", settings.skipHealthChecks)),
		trace.WithAttributes(attribute.Bool("skip_smoke_checks", settings.skipSmokeChecks)),
	)
	defer span.End()
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
				regularChecksPassed: settings.skipHealthChecks,
				machineChecksPassed: settings.skipHealthChecks,
				smokeChecksPassed:   settings.skipSmokeChecks,
			})
			machineTuples = append(machineTuples, machinePairing{oldMachine: oldMachine, newMachine: newMachine})
		}
	}

	for _, newMachine := range newMachines {
		if _, ok := oldMachines[newMachine.ID]; !ok {
			// This means we should create the new machine
			healthChecksPassed.LoadOrStore(newMachine.ID, &healthcheckResult{
				regularChecksPassed: settings.skipHealthChecks,
				machineChecksPassed: settings.skipHealthChecks,
				smokeChecksPassed:   settings.skipSmokeChecks,
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

	machineLogger := NewMachineLogger(
		map[string]statuslogger.StatusLine{},
		sl,
	)

	poolSize := md.getPoolSize(len(machineTuples))

	machineLogger.initFromMachinePairs(machineTuples)

	appStateFSM := stateless.NewStateMachine(updateNotStarted)
	appStateFSM.SetTriggerParameters(triggerUpdateMachines, reflect.TypeOf(machineTuples), reflect.TypeOf(poolSize))

	appStateFSM.Configure(updateNotStarted).Permit(triggerUpdateMachines, updatedMachines)
	appStateFSM.Configure(updatingMachines).OnEntry(func(ctx context.Context, args ...any) error {
		machPairs := args[0].([]machinePairing)
		poolSize := args[1].(int)

		return md.acquireLeases(ctx, machPairs, poolSize, machineLogger)
	}).OnExit(func(ctx context.Context, args ...any) error {
		machPairs := args[0].([]machinePairing)
		return md.releaseLeases(ctx, machPairs, machineLogger)
	})

	attempts := 0
	updateErrors := []error{}
	appStateFSM.Configure(failedUpdatingMachines).SubstateOf(updatingMachines).Permit(triggerUpdateFailed, updateFailed).Permit(triggerUpdateMachines, updatedMachines).OnEntry(func(_ context.Context, args ...any) error {
		err := args[0].(error)
		updateErrors = append(updateErrors, err)

		switch {
		case isUnrecoverableErr(err):
			return err
		case attempts > md.deployRetries:
			return err
		default:
			fmt.Fprintln(md.io.ErrOut, "Failed to update machines:", err, "Retrying...")
			time.Sleep(1 * time.Second)
			return nil
		}
	}).OnExit(func(ctx context.Context, args ...any) (err error) {
		attempts += 1
		oldAppState, err = md.appState(ctx, oldAppState)
		return err
	})
	appStateFSM.Configure(updatedMachines).SubstateOf(updatingMachines).Permit(triggerUpdateComplete, updateComplete).Permit(triggerUpdateMachinesFailed, failedUpdatingMachines).Permit(triggerUpdateFailed, updateFailed).OnEntryFrom(triggerUpdateMachines, func(_ context.Context, args ...any) error {
		machinePairs := args[0].([]machinePairing)
		machPairByProcessGroup := lo.GroupBy(machinePairs, func(machPair machinePairing) string {
			if machPair.newMachine != nil {
				return machPair.newMachine.ProcessGroup()
			} else if machPair.oldMachine != nil {
				return machPair.oldMachine.ProcessGroup()
			} else {
				return ""
			}
		})

		pgroup := errgroup.Group{}
		pgroup.SetLimit(rollingStrategyMaxConcurrentGroups)

		// We want to update by process group
		for _, machineTuples := range machPairByProcessGroup {
			machineTuples := machineTuples
			pgroup.Go(func() error {
				eg, ctx := errgroup.WithContext(ctx)

				warmMachines := lo.Filter(machineTuples, func(e machinePairing, i int) bool {
					if e.oldMachine != nil && e.oldMachine.State == "started" {
						return true
					}
					if e.newMachine != nil && e.newMachine.State == "started" {
						return true
					}
					return false
				})

				coldMachines := lo.Filter(machineTuples, func(e machinePairing, i int) bool {
					if e.oldMachine != nil && e.oldMachine.State != "started" {
						return true
					}
					if e.newMachine != nil && e.newMachine.State != "started" {
						return true
					}
					return false
				})

				eg.Go(func() (err error) {
					poolSize := len(coldMachines)
					if poolSize >= STOPPED_MACHINES_POOL_SIZE {
						poolSize = STOPPED_MACHINES_POOL_SIZE
					}

					if len(coldMachines) > 0 {
						// for cold machines, we can update all of them at once.
						// there's no need for protection against downtime since the machines are already stopped
						return md.updateProcessGroup(ctx, coldMachines, machineLogger, poolSize)
					}

					return nil
				})

				eg.Go(func() (err error) {
					// for warm machines, we update them in chunks of size, md.maxUnavailable.
					// this is to prevent downtime/low-latency during deployments
					poolSize := md.getPoolSize(len(warmMachines))
					if len(warmMachines) > 0 {
						return md.updateProcessGroup(ctx, warmMachines, machineLogger, poolSize)
					}
					return nil
				})

				err := eg.Wait()
				if err != nil {
					span.RecordError(err)
					if strings.Contains(err.Error(), "lease currently held by") {
						err = &unrecoverableError{err: err}
					}
					return err
				}

				return nil
			})
		}

		return pgroup.Wait()
	})

	appStateFSM.Configure(updateFailed).OnEntry(func(ctx context.Context, args ...any) error {
		err := args[0].(error)
		span.SetAttributes(attribute.Int("update_attempts", attempts))
		span.SetAttributes(attribute.StringSlice("update_errors", lo.Map(updateErrors, func(err error, _ int) string {
			return err.Error()
		})))

		span.RecordError(err)
		return err
	})
	appStateFSM.Configure(updateComplete).OnEntry(func(ctx context.Context, args ...any) error {
		span.SetAttributes(attribute.Int("update_attempts", attempts))
		span.SetAttributes(attribute.StringSlice("update_errors", lo.Map(updateErrors, func(err error, _ int) string {
			return err.Error()
		})))
		return nil
	})

	for {
		if updateErr := appStateFSM.FireCtx(ctx, triggerUpdateMachines, machineTuples, poolSize); updateErr != nil {
			// if we return an error when triggering machines failed, it means we're done
			if err := appStateFSM.FireCtx(ctx, triggerUpdateMachinesFailed, updateErr); err != nil {
				return appStateFSM.FireCtx(ctx, triggerUpdateFailed)
			}
		} else {
			return appStateFSM.FireCtx(ctx, triggerUpdateComplete, machineTuples)
		}
	}
}

func (md *machineDeployment) updateProcessGroup(ctx context.Context, machineTuples []machinePairing, machineLogger *MachineLogger, poolSize int) error {
	ctx, span := tracing.GetTracer().Start(ctx, "update_process_group")
	defer span.End()

	group := errgroup.Group{}
	group.SetLimit(poolSize)

	for _, machPair := range machineTuples {
		machPair := machPair
		oldMachine := machPair.oldMachine
		newMachine := machPair.newMachine

		group.Go(func() error {
			checkResult, _ := healthChecksPassed.Load(machPair.oldMachine.ID)
			machineCheckResult := checkResult.(*healthcheckResult)

			var sl statuslogger.StatusLine
			if oldMachine != nil {
				sl = machineLogger.getLoggerFromID(oldMachine.ID)
			} else if newMachine != nil {
				sl = machineLogger.getLoggerFromID(newMachine.ID)
			}

			err := md.updateMachineWChecks(ctx, oldMachine, newMachine, sl, md.io, machineCheckResult)
			if err != nil {
				sl.LogStatus(statuslogger.StatusFailure, err.Error())
				span.RecordError(err)
				return fmt.Errorf("failed to update machine %s: %w", oldMachine.ID, err)
			}
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		span.RecordError(err)
		return err
	}

	return nil
}

func (md *machineDeployment) acquireLeases(ctx context.Context, machineTuples []machinePairing, poolSize int, machToLogger *MachineLogger) error {
	ctx, span := tracing.GetTracer().Start(ctx, "acquire_leases")

	leaseGroup := errgroup.Group{}
	leaseGroup.SetLimit(poolSize)

	for _, machineTuple := range machineTuples {
		machineTuple := machineTuple
		leaseGroup.Go(func() error {

			var machine *fly.Machine
			if machineTuple.oldMachine != nil {
				machine = machineTuple.oldMachine
			} else if machineTuple.newMachine != nil {
				machine = machineTuple.newMachine
			} else {
				return nil
			}
			sl := machToLogger.getLoggerFromID(machine.ID)

			if machine.HostStatus == fly.HostStatusUnreachable {
				sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Skipped lease for unreachable machine %s", machine.ID))
				return nil
			}

			if machine.LeaseNonce != "" {
				sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Already have lease for %s", machine.ID))
				return nil
			}

			sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Acquiring lease for %s", machine.ID))

			lease, err := md.acquireMachineLease(ctx, machine.ID)
			if err != nil {
				sl.LogStatus(statuslogger.StatusFailure, fmt.Sprintf("Failed to acquire lease for %s: %v", machine.ID, err))
				return err
			}

			machine.LeaseNonce = lease.Data.Nonce
			lm := mach.NewLeasableMachine(md.flapsClient, md.io, machine, false)
			lm.StartBackgroundLeaseRefresh(ctx, md.leaseTimeout, md.leaseDelayBetween)
			sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Acquired lease for %s", machine.ID))
			return nil
		})
	}

	if err := leaseGroup.Wait(); err != nil {
		span.RecordError(err)
		return err
	}

	return nil
}

func (md *machineDeployment) releaseLeases(ctx context.Context, machineTuples []machinePairing, machToLogger *MachineLogger) error {
	ctx = context.WithoutCancel(ctx)
	ctx, span := tracing.GetTracer().Start(ctx, "release_leases")
	defer span.End()

	leaseGroup := errgroup.Group{}
	leaseGroup.SetLimit(len(machineTuples))

	for _, machineTuple := range machineTuples {
		machineTuple := machineTuple

		leaseGroup.Go(func() error {

			var machine *fly.Machine
			if machineTuple.oldMachine != nil {
				machine = machineTuple.oldMachine
			} else if machineTuple.newMachine != nil {
				machine = machineTuple.newMachine
			} else {
				return nil
			}

			sl := machToLogger.getLoggerFromID(machine.ID)

			sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Clearing lease for %s", machine.ID))
			if machine.LeaseNonce == "" {
				sl.LogStatus(statuslogger.StatusSuccess, fmt.Sprintf("Cleared lease for %s", machine.ID))
				return nil
			}
			err := md.clearMachineLease(ctx, machine.ID, machine.LeaseNonce)
			if err != nil {
				sl.LogStatus(statuslogger.StatusFailure, fmt.Sprintf("Failed to clear lease for %s: %v", machine.ID, err))
				return err
			}

			sl.LogStatus(statuslogger.StatusSuccess, fmt.Sprintf("Cleared lease for %s", machine.ID))
			return nil
		})
	}

	if err := leaseGroup.Wait(); err != nil {
		span.RecordError(err)
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

func compareConfigs(ctx context.Context, oldConfig, newConfig *fly.MachineConfig) bool {
	_, span := tracing.GetTracer().Start(ctx, "compare_configs")
	defer span.End()

	opt := cmp.FilterPath(func(p cmp.Path) bool {
		vx := p.Last().String()

		// ignore the flyctl version used for the deployment. this is mostly useful for testing
		if vx == `["fly_flyctl_version"]` {
			return true
		}
		return false
	}, cmp.Ignore())

	isEqual := cmp.Equal(oldConfig, newConfig, opt)
	span.SetAttributes(attribute.Bool("configs_equal", isEqual))
	return isEqual
}

func (md *machineDeployment) updateMachineWChecks(ctx context.Context, oldMachine, newMachine *fly.Machine, sl statuslogger.StatusLine, io *iostreams.IOStreams, healthcheckResult *healthcheckResult) error {
	ctx, span := tracing.GetTracer().Start(ctx, "update_machine_w_checks", trace.WithAttributes(
		attribute.Bool("smoke_checks", healthcheckResult.smokeChecksPassed),
		attribute.Bool("machine_checks", healthcheckResult.machineChecksPassed),
		attribute.Bool("regular_checks", healthcheckResult.regularChecksPassed),
	))
	defer span.End()

	var machine *fly.Machine
	var lease *fly.MachineLease

	var err error

	machine, lease, err = md.updateOrCreateMachine(ctx, oldMachine, newMachine, sl)
	// if machine is nil and the lease is nil, it means we don't need to check on this machine
	if err != nil || (machine == nil && lease == nil) {
		span.RecordError(err)
		return err
	}

	lm := mach.NewLeasableMachine(md.flapsClient, io, machine, false)

	shouldStart := lo.Contains([]string{"started", "replacing"}, newMachine.State)
	span.SetAttributes(attribute.Bool("should_start", shouldStart))

	if !shouldStart {
		sl.LogStatus(statuslogger.StatusSuccess, fmt.Sprintf("Machine %s is now in a good state", machine.ID))
		return nil
	}

	if !healthcheckResult.machineChecksPassed || !healthcheckResult.smokeChecksPassed {
		sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Waiting for machine %s to reach a good state", oldMachine.ID))
		_, err := waitForMachineState(ctx, lm, []string{"stopped", "started", "suspended"}, md.waitTimeout, sl)
		if err != nil {
			span.RecordError(err)
			return err
		}
	}

	md.warnAboutIncorrectListenAddress(ctx, lm)

	if !healthcheckResult.smokeChecksPassed {
		sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Running smoke checks on machine %s", machine.ID))
		err = md.doSmokeChecks(ctx, lm, false)
		if err != nil {
			span.RecordError(err)
			return &unrecoverableError{err: err}
		}
		healthcheckResult.smokeChecksPassed = true
	}

	if !healthcheckResult.machineChecksPassed {
		sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Running machine checks on machine %s", machine.ID))
		err = md.runTestMachines(ctx, machine, sl)
		if err != nil {
			err := &unrecoverableError{err: err}
			span.RecordError(err)
			return err
		}
		healthcheckResult.machineChecksPassed = true
	}

	if !healthcheckResult.regularChecksPassed {
		sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Checking health of machine %s", machine.ID))
		err = lm.WaitForHealthchecksToPass(ctx, md.waitTimeout)
		if err != nil {
			err := &unrecoverableError{err: err}
			span.RecordError(err)
			return err
		}
		healthcheckResult.regularChecksPassed = true
	}

	sl.LogStatus(statuslogger.StatusSuccess, fmt.Sprintf("Machine %s is now in a good state", machine.ID))

	return nil
}

func (md *machineDeployment) updateOrCreateMachine(ctx context.Context, oldMachine, newMachine *fly.Machine, sl statuslogger.StatusLine) (*fly.Machine, *fly.MachineLease, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "update_or_create_machine")
	defer span.End()

	if oldMachine != nil {
		span.AddEvent("Old machine exists")
		if newMachine == nil {
			span.AddEvent("Destroying old machine")
			sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Destroying machine %s", oldMachine.ID))

			err := md.destroyMachine(ctx, oldMachine.ID, oldMachine.LeaseNonce)
			span.RecordError(err)

			sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Destroyed machine %s", oldMachine.ID))
			return nil, nil, err
		} else {
			span.AddEvent("Updating old machine")
			sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Updating machine config for %s", oldMachine.ID))
			machine, err := md.updateMachineConfig(ctx, oldMachine, newMachine.Config, sl, newMachine.State == "replacing")
			if err != nil {
				span.RecordError(err)
				return oldMachine, nil, err
			}
			sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Updated machine config for %s", oldMachine.ID))

			return machine, nil, nil
		}
	} else if newMachine != nil {
		span.AddEvent("Creating a new machine")
		sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Creating machine for %s", newMachine.ID))
		machine, err := md.createMachine(ctx, newMachine.Config, newMachine.Region)
		if err != nil {
			span.RecordError(err)
			return nil, nil, err
		}

		sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Acquiring lease for %s", newMachine.ID))
		lease, err := md.acquireMachineLease(ctx, machine.ID)
		if err != nil {
			span.RecordError(err)
			return nil, nil, err
		}
		sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Acquired lease for %s", newMachine.ID))

		return machine, lease, nil
	} else {
		// both old and new machines are nil, so just a noop
		return nil, nil, nil
	}
}

func (md *machineDeployment) destroyMachine(ctx context.Context, machineID string, lease string) error {
	err := md.flapsClient.Destroy(ctx, fly.RemoveMachineInput{
		ID:   machineID,
		Kill: true,
	}, lease)
	if err != nil {
		return err
	}

	return nil
}

func (md *machineDeployment) clearMachineLease(ctx context.Context, machID, leaseNonce string) error {
	// TODO: remove this when the flaps retry work is done
	attempts := 0
	for {
		err := md.flapsClient.ReleaseLease(ctx, machID, leaseNonce)
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
	ctx, span := tracing.GetTracer().Start(ctx, "wait_for_machine_state", trace.WithAttributes(
		attribute.StringSlice("possible_states", possibleStates),
	))
	defer span.End()

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
			if successfulState != "" {
				span.SetAttributes(attribute.String("state", successfulState))
			}

			if waitErr != nil {
				span.RecordError(waitErr)
			}

			return successfulState, waitErr
		}
		mutex.Unlock()

		time.Sleep(1 * time.Second)
	}
}

func (md *machineDeployment) acquireMachineLease(ctx context.Context, machID string) (*fly.MachineLease, error) {
	leaseTimeout := int(md.leaseTimeout)
	lease, err := md.flapsClient.AcquireLease(ctx, machID, &leaseTimeout)
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
	ctx, span := tracing.GetTracer().Start(ctx, "update_machine_config")
	defer span.End()
	if compareConfigs(ctx, oldMachine.Config, newMachineConfig) {
		return oldMachine, nil
	}

	input, err := md.launchInputForUpdate(oldMachine)
	if err != nil {
		return nil, err
	}
	input.Config = newMachineConfig
	input.RequiresReplacement = input.RequiresReplacement || shouldReplace

	lm := mach.NewLeasableMachine(md.flapsClient, md.io, oldMachine, false)
	entry := &machineUpdateEntry{
		leasableMachine: lm,
		launchInput:     input,
	}
	err = md.updateMachine(ctx, entry, sl)
	if err != nil {
		return nil, err
	}
	return entry.leasableMachine.Machine(), nil
}

func (md *machineDeployment) createMachine(ctx context.Context, machConfig *fly.MachineConfig, region string) (*fly.Machine, error) {
	machine, err := md.flapsClient.Launch(ctx, fly.LaunchMachineInput{
		Config: machConfig,
		Region: region,
	})
	if err != nil {
		return nil, err
	}

	return machine, nil
}

func isUnrecoverableErr(err error) bool {
	var unrecoverableErr *unrecoverableError
	switch {
	case errors.As(err, &unrecoverableErr):
		return true
	case errors.Is(err, context.Canceled):
		return true
	default:
		return false
	}
}
