package deploy

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/go-cmp/cmp"
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

type AppState struct {
	Machines []*fly.Machine
}

type machinePairing struct {
	// plannedID remains stable when the API assigns a replacement a new ID.
	plannedID string
	// plannedState keeps an accepted replacement in the lane its plan chose.
	plannedState string
	// originalMachine is this machine as read before the deploy began. It's not re-read on recovery retries.
	// nil for machines being created during the deploy.
	originalMachine *fly.Machine
	oldMachine      *fly.Machine
	newMachine      *fly.Machine
}

// appState returns the app's state from Flaps.
func (md *machineDeployment) appState(ctx context.Context, existingAppState *AppState) (*AppState, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "app_state")
	defer span.End()

	machines, err := md.flapsClient.List(ctx, md.app.Name, "")
	if err != nil {
		tracing.RecordErrorEvent(ctx, span, err)

		return nil, err
	}

	if existingAppState != nil {
		for _, machine := range machines {
			if existingMachine, ok := lo.Find(existingAppState.Machines, func(m *fly.Machine) bool {
				return m.ID == machine.ID
			}); ok {
				machine.LeaseNonce = existingMachine.LeaseNonce
			}
		}
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

type updateMachineSettings struct {
	pushForward      bool
	skipHealthChecks bool
	skipSmokeChecks  bool
	outerMachines    map[string]*fly.Machine
}

const rollingStrategyMaxConcurrentGroups = 16

func (md *machineDeployment) updateMachinesWRecovery(ctx context.Context, originalAppState, oldAppState, newAppState *AppState, statusLogger statuslogger.StatusLogger, settings updateMachineSettings) error {
	ctx, span := tracing.GetTracer().Start(
		ctx, "update_machines_w_recovery",
		trace.WithAttributes(attribute.Bool("push_forward", settings.pushForward)),
		trace.WithAttributes(attribute.Bool("skip_health_checks", settings.skipHealthChecks)),
		trace.WithAttributes(attribute.Bool("skip_smoke_checks", settings.skipSmokeChecks)),
	)
	defer span.End()
	ctx, cancelCause := context.WithCancelCause(ctx)
	cancel := func() { cancelCause(fmt.Errorf("machine updates finished: %w", context.Canceled)) }
	ctx, cancel = ctrlc.HookCancelableContext(ctx, cancel)
	defer cancel()

	originalMachines := make(map[string]*fly.Machine)
	if originalAppState != nil {
		for _, machine := range originalAppState.Machines {
			originalMachines[machine.ID] = machine
		}
	}
	oldMachines := make(map[string]*fly.Machine)
	for _, machine := range oldAppState.Machines {
		oldMachines[machine.ID] = machine
	}
	machineTuples := make([]machinePairing, 0, len(newAppState.Machines))
	for _, planned := range newAppState.Machines {
		target := planned
		if accepted, ok := md.recoveryMachines.Load(planned.ID); ok {
			target = accepted.(*fly.Machine)
			if oldMachines[target.ID] == nil {
				// A phase refresh can omit a recently created replacement.
				current, err := md.flapsClient.Get(ctx, md.app.Name, target.ID)
				if err != nil {
					return fmt.Errorf("refresh accepted Machine %s for %s: %w", target.ID, planned.ID, err)
				}
				if current == nil || current.ID != target.ID {
					return fmt.Errorf("could not confirm accepted Machine %s for %s", target.ID, planned.ID)
				}
				// A GET confirms identity but does not grant a lease to this attempt.
				refreshed := *current
				refreshed.LeaseNonce = ""
				oldMachines[target.ID] = &refreshed
			}
			// Only the outer attempt can supply an owned lease for this exact ID.
			if outer := settings.outerMachines[target.ID]; outer != nil {
				oldMachines[target.ID].LeaseNonce = outer.LeaseNonce
			}
		}
		healthChecksPassed.LoadOrStore(planned.ID, &healthcheckResult{
			regularChecksPassed: settings.skipHealthChecks,
			smokeChecksPassed:   settings.skipSmokeChecks,
			machineChecksPassed: settings.skipHealthChecks,
		})
		machineTuples = append(machineTuples, machinePairing{
			plannedID:       planned.ID,
			plannedState:    planned.State,
			originalMachine: originalMachines[planned.ID],
			oldMachine:      oldMachines[target.ID],
			newMachine:      target,
		})
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

	machineLogger.initFromMachinePairs(machineTuples)

	machPairByProcessGroup := lo.GroupBy(machineTuples, func(machPair machinePairing) string {
		if machPair.oldMachine != nil {
			return machPair.oldMachine.ProcessGroup()
		} else if machPair.newMachine != nil {
			return machPair.newMachine.ProcessGroup()
		} else {
			return ""
		}
	})

	poolSize := md.getPoolSize(len(machineTuples))

	leaseTuples := machineTuples
	if settings.outerMachines != nil {
		// Original Machines retain their outer attempt's leases. Accepted
		// replacements have different IDs and need leases of their own.
		leaseTuples = lo.Filter(machineTuples, func(pair machinePairing, _ int) bool {
			return pair.oldMachine != nil && pair.oldMachine.ID != pair.plannedID && pair.oldMachine.LeaseNonce == ""
		})
	}
	if len(leaseTuples) > 0 {
		attempts := 0

		defer func() {
			span.SetAttributes(attribute.Int("lease_attempts", attempts))
		}()

		defer func() {
			err := md.releaseLeases(ctx, leaseTuples, machineLogger)
			if err != nil {
				fmt.Fprintln(md.io.ErrOut, "Failed to release leases:", err)
				tracing.RecordErrorEvent(ctx, span, err)
			}
		}()

		for {
			err := md.acquireLeases(ctx, leaseTuples, poolSize, machineLogger)
			if err == nil {
				break
			}
			attempts += 1

			var unrecoverableErr *unrecoverableError
			if attempts > md.deployRetries || errors.As(err, &unrecoverableErr) || errors.Is(err, context.Canceled) {
				tracing.RecordErrorEvent(ctx, span, err)

				return fmt.Errorf("failed to acquire leases: %w", err)
			}
		}
	}

	pgroup := errgroup.Group{}
	pgroup.SetLimit(rollingStrategyMaxConcurrentGroups)

	// We want to update by process group
	for _, machineTuples := range machPairByProcessGroup {
		pgroup.Go(func() error {
			eg, ctx := errgroup.WithContext(ctx)

			isWarm := func(e machinePairing, i int) bool {
				if e.plannedState == "started" || e.plannedState == "replacing" {
					return true
				}
				if e.oldMachine != nil && (e.oldMachine.State == "started" || e.oldMachine.State == "replacing") {
					return true
				}
				if e.newMachine != nil && (e.newMachine.State == "started" || e.newMachine.State == "replacing") {
					return true
				}

				return false
			}
			warmMachines := lo.Filter(machineTuples, isWarm)
			coldMachines := lo.Reject(machineTuples, isWarm)

			eg.Go(func() (err error) {
				poolSize := min(len(coldMachines), STOPPED_MACHINES_POOL_SIZE)

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
				tracing.RecordErrorEvent(ctx, span, err)
				if strings.Contains(err.Error(), "lease currently held by") {
					err = &unrecoverableError{err: err}
				}

				return err
			}

			return nil
		})
	}

	attempts := 0
	if updateErr := pgroup.Wait(); updateErr != nil {
		var unrecoverableErr *unrecoverableError
		if !settings.pushForward || errors.As(updateErr, &unrecoverableErr) || errors.Is(updateErr, context.Canceled) {
			tracing.RecordErrorEvent(ctx, span, updateErr)

			return updateErr
		}

		// if we fail to update the machines, we should push the state forward if possible
		for {
			defer func() {
				span.SetAttributes(attribute.Int("update_attempts", attempts))
			}()

			if attempts > md.deployRetries {
				fmt.Fprintln(md.io.ErrOut, "Failed to update machines:", updateErr)
				tracing.RecordErrorEvent(ctx, span, updateErr)

				return updateErr
			}

			currentState, err := md.appState(ctx, oldAppState)
			if err != nil {
				tracing.RecordErrorEvent(ctx, span, updateErr)

				return fmt.Errorf("failed to get current app state: %w", err)
			}
			if md.waitForReplacingMachinesToSettle(ctx, currentState, machineLogger) {
				currentState, err = md.appState(ctx, currentState)
				if err != nil {
					tracing.RecordErrorEvent(ctx, span, err)

					return fmt.Errorf("failed to refresh app state after waiting for replaces: %w", err)
				}
			}
			// sort machines by id so we always have the same order when retrying
			// This is needed for rolling deploys so we can start from the same machine
			sort.Slice(currentState.Machines, func(i, j int) bool {
				return currentState.Machines[i].ID < currentState.Machines[j].ID
			})
			err = md.updateMachinesWRecovery(ctx, originalAppState, currentState, newAppState, sl, updateMachineSettings{
				pushForward:      false,
				skipHealthChecks: settings.skipHealthChecks,
				skipSmokeChecks:  settings.skipSmokeChecks,
				outerMachines:    oldMachines,
			})
			if err == nil {
				break
			} else if errors.Is(err, context.Canceled) {
				tracing.RecordErrorEvent(ctx, span, updateErr)

				return err
			} else {
				if errors.As(err, &unrecoverableErr) {
					tracing.RecordErrorEvent(ctx, span, updateErr)

					return err
				}
				fmt.Fprintln(md.io.ErrOut, "Failed to update machines:", err, "Retrying...")
			}
			attempts++
			if attempts <= md.deployRetries {
				select {
				case <-time.After(retryBackoff(attempts)):
				case <-ctx.Done():
					return context.Cause(ctx)
				}
			}
		}
	}

	return nil
}

func (md *machineDeployment) updateProcessGroup(ctx context.Context, machineTuples []machinePairing, machineLogger *MachineLogger, poolSize int) error {
	ctx, span := tracing.GetTracer().Start(ctx, "update_process_group")
	defer span.End()

	group, gCtx := errgroup.WithContext(ctx)
	group.SetLimit(poolSize)

	for _, machPair := range machineTuples {
		originalMachine := machPair.originalMachine
		oldMachine := machPair.oldMachine
		newMachine := machPair.newMachine

		group.Go(func() error {
			// if both old and new machines are nil, we don't need to update anything
			if oldMachine == nil && newMachine == nil {
				span.AddEvent("Both old and new machines are nil")

				return nil
			}

			// Determine if the machine should be started after deployment. This is based
			// on the state of originalMachine, which was captured pre-deploy and won't
			// change during deploy recovery retries.
			skipLaunch := false
			if newMachine != nil {
				skipLaunch = shouldSkipLaunch(originalMachine, newMachine.Config)
			}

			var machineID string
			if oldMachine != nil {
				machineID = oldMachine.ID
			} else {
				machineID = newMachine.ID
			}

			sl := machineLogger.getLoggerFromID(machineID)

			if err := gCtx.Err(); err != nil {
				sl.LogStatus(statuslogger.StatusFailure, "skipping machine update due to earlier failure")

				return err
			}

			checkResult, ok := healthChecksPassed.Load(machPair.plannedID)
			// this shouldn't happen, we ensure that the machine is in the map but just in case
			if !ok {
				err := fmt.Errorf("no health checks stored for machine")
				sl.LogStatus(statuslogger.StatusFailure, err.Error())
				tracing.RecordErrorEvent(gCtx, span, err)

				return fmt.Errorf("failed to update machine %s: %w", machineID, err)
			}
			machineCheckResult := checkResult.(*healthcheckResult)

			err := md.updateMachineWChecks(gCtx, oldMachine, newMachine, skipLaunch, sl, md.io, machineCheckResult, machPair.plannedID)
			if err != nil {
				sl.LogStatus(statuslogger.StatusFailure, err.Error())
				tracing.RecordErrorEvent(gCtx, span, err)

				return fmt.Errorf("failed to update machine %s: %w", machineID, err)
			}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		tracing.RecordErrorEvent(gCtx, span, err)

		return err
	}

	return nil
}

func (md *machineDeployment) acquireLeases(ctx context.Context, machineTuples []machinePairing, poolSize int, machToLogger *MachineLogger) error {
	ctx, span := tracing.GetTracer().Start(ctx, "acquire_leases")
	defer span.End()

	leaseGroup := errgroup.Group{}
	if poolSize <= 0 {
		panic("pool size must be > 0")
	}
	leaseGroup.SetLimit(poolSize)

	for _, machineTuple := range machineTuples {
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
			lm := mach.NewLeasableMachine(md.flapsClient, md.io, md.app.Name, machine, false)
			lm.StartBackgroundLeaseRefresh(ctx, md.leaseTimeout, md.leaseDelayBetween)
			sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Acquired lease for %s", machine.ID))

			return nil
		})
	}

	if err := leaseGroup.Wait(); err != nil {
		tracing.RecordError(ctx, span, err, "failed to acquire leases")

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

		leaseGroup.Go(func() error {

			var machine *fly.Machine
			if machineTuple.oldMachine != nil {
				machine = machineTuple.oldMachine
			} else if machineTuple.newMachine != nil {
				machine = machineTuple.newMachine
			} else {
				return nil
			}

			if machine.LeaseNonce == "" {
				return nil
			}
			sl := machToLogger.getLoggerFromID(machine.ID)
			sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Clearing lease for %s", machine.ID))
			err := md.clearMachineLease(ctx, machine.ID, machine.LeaseNonce)
			if err != nil {
				sl.LogStatus(statuslogger.StatusFailure, fmt.Sprintf("Failed to clear lease for %s: %v", machine.ID, err))

				return err
			}
			machine.LeaseNonce = ""

			sl.LogStatus(statuslogger.StatusSuccess, fmt.Sprintf("Cleared lease for %s", machine.ID))

			return nil
		})
	}

	if err := leaseGroup.Wait(); err != nil {
		tracing.RecordErrorEvent(ctx, span, err)

		return err
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

	opt := cmp.Options{
		cmp.FilterPath(func(p cmp.Path) bool {
			vx := p.Last().String()

			// ignore the flyctl version used for the deployment. this is mostly useful for testing
			if vx == `["fly_flyctl_version"]` {
				return true
			}

			return false
		}, cmp.Ignore()),
		// Treat nil slices and empty slices as equal to avoid spurious diffs
		// from JSON roundtripping (e.g. API returns [] where flyctl sent nil).
		cmp.FilterValues(func(x, y interface{}) bool {
			return isEmptyOrNilSlice(x) && isEmptyOrNilSlice(y)
		}, cmp.Ignore()),
	}

	isEqual := cmp.Equal(oldConfig, newConfig, opt)
	span.SetAttributes(attribute.Bool("configs_equal", isEqual))

	return isEqual
}

func isEmptyOrNilSlice(v interface{}) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)

	return rv.Kind() == reflect.Slice && rv.Len() == 0
}

func (md *machineDeployment) updateMachineWChecks(ctx context.Context, oldMachine, newMachine *fly.Machine, skipLaunch bool, sl statuslogger.StatusLine, io *iostreams.IOStreams, healthcheckResult *healthcheckResult, plannedID string) error {
	ctx, span := tracing.GetTracer().Start(ctx, "update_machine_w_checks", trace.WithAttributes(
		attribute.Bool("smoke_checks", healthcheckResult.smokeChecksPassed),
		attribute.Bool("machine_checks", healthcheckResult.machineChecksPassed),
		attribute.Bool("regular_checks", healthcheckResult.regularChecksPassed),
	))
	defer span.End()

	var machine *fly.Machine
	var lease *fly.MachineLease

	var err error

	machine, lease, err = md.updateOrCreateMachine(ctx, oldMachine, newMachine, skipLaunch, sl)
	if machine != nil && machine.ID != plannedID && (err == nil || oldMachine == nil) {
		// Keep the accepted identity and server-resolved config even if readiness
		// fails next. The old ID and unresolved volume selection cannot be reused.
		accepted := *machine
		// Replacement leases are released by updateMachineByReplace. Recovery
		// must acquire its own lease instead of carrying a nonce to another ID.
		accepted.LeaseNonce = ""
		md.recoveryMachines.Store(plannedID, &accepted)
	}

	// if machine is nil and the lease is nil, it means we don't need to check on this machine
	if err != nil || (machine == nil && lease == nil) {
		tracing.RecordErrorEvent(ctx, span, err)

		return err
	}

	checkErr := md.checkUpdatedMachine(ctx, oldMachine, machine, skipLaunch, sl, io, healthcheckResult)
	if lease != nil {
		// A fallback create owns this lease only for its readiness checks.
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if releaseErr := md.flapsClient.ReleaseLease(releaseCtx, md.app.Name, machine.ID, lease.Data.Nonce); releaseErr != nil && !strings.Contains(releaseErr.Error(), "lease not found") {
			fmt.Fprintf(md.io.ErrOut, "Failed to release create lease on %s: %v\n", machine.ID, releaseErr)
		}
	}

	return checkErr
}

func (md *machineDeployment) checkUpdatedMachine(ctx context.Context, oldMachine, machine *fly.Machine, skipLaunch bool, sl statuslogger.StatusLine, io *iostreams.IOStreams, healthcheckResult *healthcheckResult) (err error) {
	ctx, span := tracing.GetTracer().Start(ctx, "deployment_readiness", trace.WithAttributes(
		attribute.String("machine.ID", machine.ID),
		attribute.Bool("deployment.skip_health_checks", md.skipHealthChecks),
		attribute.Bool("deployment.skip_smoke_checks", md.skipSmokeChecks),
	))
	defer func() {
		tracing.RecordError(ctx, span, err, "machine readiness failed")
		span.End()
	}()

	lm := mach.NewLeasableMachine(md.flapsClient, io, md.app.Name, machine, false)

	span.SetAttributes(attribute.Bool("should_start", !skipLaunch))

	if !healthcheckResult.machineChecksPassed || !healthcheckResult.smokeChecksPassed {
		sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Waiting for machine %s to reach a good state", machine.ID))
		state, err := waitForMachineState(ctx, lm, []string{"stopped", "started", "suspended"}, md.waitTimeout, sl)
		if err != nil {
			tracing.RecordErrorEvent(ctx, span, err)

			return err
		}
		if state == fly.MachineStateStopped && oldMachine != nil &&
			supportsPreservedStoppedUpdate(md.strategy, oldMachine.State) &&
			md.readPreservedStoppedUpdate(ctx, machine.ID, machine.Version) {
			sl.LogStatus(statuslogger.StatusSuccess, fmt.Sprintf("Machine %s was updated and left stopped", machine.ID))

			return nil
		}
	}

	if skipLaunch {
		sl.LogStatus(statuslogger.StatusSuccess, fmt.Sprintf("Machine %s is now in a good state", machine.ID))

		return nil
	}

	md.warnAboutIncorrectListenAddress(ctx, lm)

	if !healthcheckResult.smokeChecksPassed {
		sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Running smoke checks on machine %s", machine.ID))
		err = md.doSmokeChecks(ctx, lm, false)
		if err != nil {
			tracing.RecordErrorEvent(ctx, span, err)

			return &unrecoverableError{err: err}
		}
		healthcheckResult.smokeChecksPassed = true
	}

	if !healthcheckResult.machineChecksPassed {
		sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Running machine checks on machine %s", machine.ID))
		err = md.runTestMachines(ctx, machine, sl)
		if err != nil {
			err := &unrecoverableError{err: err}
			tracing.RecordErrorEvent(ctx, span, err)

			return err
		}
		healthcheckResult.machineChecksPassed = true
	}

	if !healthcheckResult.regularChecksPassed {
		sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Checking health of machine %s", machine.ID))
		err = lm.WaitForHealthchecksToPass(ctx, md.waitTimeout)
		if err != nil {
			err := &unrecoverableError{err: err}
			tracing.RecordErrorEvent(ctx, span, err)

			return err
		}
		healthcheckResult.regularChecksPassed = true
	}

	sl.LogStatus(statuslogger.StatusSuccess, fmt.Sprintf("Machine %s is now in a good state", machine.ID))

	return nil
}

func (md *machineDeployment) updateOrCreateMachine(ctx context.Context, oldMachine, newMachine *fly.Machine, skipLaunch bool, sl statuslogger.StatusLine) (*fly.Machine, *fly.MachineLease, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "update_or_create_machine")
	defer span.End()

	if oldMachine != nil {
		span.AddEvent("Old machine exists")
		if newMachine == nil {
			span.AddEvent("Destroying old machine")
			sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Destroying machine %s", oldMachine.ID))

			err := md.destroyMachine(ctx, oldMachine.ID, oldMachine.LeaseNonce)
			tracing.RecordErrorEvent(ctx, span, err)

			sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Destroyed machine %s", oldMachine.ID))

			return nil, nil, err
		} else {
			span.AddEvent("Updating old machine")
			sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Updating machine config for %s", oldMachine.ID))
			machine, err := md.updateMachineConfig(ctx, oldMachine, newMachine.Config, sl, newMachine.State == "replacing", skipLaunch)
			if err != nil {
				tracing.RecordErrorEvent(ctx, span, err)

				return oldMachine, nil, err
			}
			sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Updated machine config for %s", oldMachine.ID))

			return machine, nil, nil
		}
	} else if newMachine != nil {
		span.AddEvent("Creating a new machine")
		sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Creating machine for %s", newMachine.ID))
		machine, err := md.createMachine(ctx, newMachine.Config, newMachine.Region, skipLaunch)
		if err != nil {
			tracing.RecordErrorEvent(ctx, span, err)

			return nil, nil, err
		}

		sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Acquiring lease for %s", newMachine.ID))
		lease, err := md.acquireMachineLease(ctx, machine.ID)
		if err != nil {
			tracing.RecordErrorEvent(ctx, span, err)

			// Launch succeeded. Recovery still needs its ID when leasing fails.
			return machine, nil, err
		}
		sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Acquired lease for %s", newMachine.ID))

		return machine, lease, nil
	} else {
		// both old and new machines are nil, so just a noop
		return nil, nil, nil
	}
}

func (md *machineDeployment) destroyMachine(ctx context.Context, machineID string, lease string) error {
	err := md.flapsClient.Destroy(ctx, md.app.Name, fly.RemoveMachineInput{
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
		err := md.flapsClient.ReleaseLease(ctx, md.app.Name, machID, leaseNonce)
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

	ctx, cancel := context.WithTimeoutCause(ctx, timeout, fmt.Errorf("waiting for deployment machine state: %w", context.DeadlineExceeded))
	defer cancel()

	var mutex sync.Mutex

	var waitErr error
	numCompleted := 0
	var successfulState string

	for _, state := range possibleStates {
		go func() {
			err := lm.WaitForState(ctx, state, timeout)
			mutex.Lock()
			defer func() {
				numCompleted += 1
				mutex.Unlock()
			}()

			if successfulState != "" {
				return
			}
			if err != nil {
				waitErr = err
			} else {
				sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Machine %s reached %s state", lm.Machine().ID, state))
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

				return successfulState, nil
			}

			if waitErr != nil {
				tracing.RecordErrorEvent(ctx, span, waitErr)
			}

			return successfulState, waitErr
		}
		mutex.Unlock()

		time.Sleep(1 * time.Second)
	}
}

// waitForReplacingMachinesToSettle gives in-flight updates up to waitTimeout to
// finish. It reports whether the caller should refresh its Machine snapshot.
// The next update attempt handles Machines that are still replacing.
func (md *machineDeployment) waitForReplacingMachinesToSettle(ctx context.Context, state *AppState, machineLogger *MachineLogger) bool {
	stableStates := []string{fly.MachineStateStarted, fly.MachineStateStopped, fly.MachineStateSuspended}
	var wg sync.WaitGroup
	waited := false
	for _, m := range state.Machines {
		if m.State != "replacing" {
			continue
		}
		sl := machineLogger.getLoggerFromID(m.ID)
		if sl == nil {
			// Not a machine we're tracking in this deploy; nothing to log against.
			continue
		}

		waited = true
		wg.Go(func() {
			sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Waiting for in-flight replace of %s to settle", m.ID))
			lm := mach.NewLeasableMachine(md.flapsClient, md.io, md.app.Name, m, false)
			_, _ = waitForMachineState(ctx, lm, stableStates, md.waitTimeout, sl)
		})
	}
	wg.Wait()
	return waited
}

// retryBackoff returns the delay before the Nth (1-based) recovery retry. It grows
// exponentially and is capped so a run of retries can't stall a deploy indefinitely.
func retryBackoff(attempt int) time.Duration {
	const base = 1 * time.Second
	const maxBackoff = 15 * time.Second

	if attempt < 1 {
		attempt = 1
	}
	return min(base<<min(attempt-1, 4), maxBackoff)
}

func (md *machineDeployment) acquireMachineLease(ctx context.Context, machID string) (*fly.MachineLease, error) {
	leaseTimeout := int(md.leaseTimeout.Seconds())
	lease, err := md.flapsClient.AcquireLease(ctx, md.app.Name, machID, &leaseTimeout)
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

func (md *machineDeployment) updateMachineConfig(ctx context.Context, oldMachine *fly.Machine, newMachineConfig *fly.MachineConfig, sl statuslogger.StatusLine, shouldReplace bool, skipLaunch bool) (*fly.Machine, error) {
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
	input.SkipLaunch = skipLaunch

	lm := mach.NewLeasableMachine(md.flapsClient, md.io, md.app.Name, oldMachine, false)
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

func (md *machineDeployment) createMachine(ctx context.Context, machConfig *fly.MachineConfig, region string, skipLaunch bool) (*fly.Machine, error) {
	machine, err := md.flapsClient.Launch(ctx, md.app.Name, fly.LaunchMachineInput{
		Config:     machConfig,
		Region:     region,
		SkipLaunch: skipLaunch,
	})
	if err != nil {
		return nil, err
	}

	return machine, nil
}
