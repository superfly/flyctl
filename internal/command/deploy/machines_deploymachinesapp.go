package deploy

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/miekg/dns"
	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
	machcmd "github.com/superfly/flyctl/internal/command/machine"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/statuslogger"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/maps"
	"golang.org/x/sync/errgroup"
)

const rollingStrategyMaxConcurrentGroups = 10

type ProcessGroupsDiff struct {
	machinesToRemove      []machine.LeasableMachine
	groupsToRemove        map[string]int
	groupsNeedingMachines map[string]bool
}

func (md *machineDeployment) DeployMachinesApp(ctx context.Context) error {
	ctx, span := tracing.GetTracer().Start(ctx, "deploy_machines")
	defer span.End()

	ctx = flapsutil.NewContextWithClient(ctx, md.flapsClient)

	onInterruptContext := context.WithoutCancel(ctx)

	if err := md.updateReleaseInBackend(ctx, "running"); err != nil {
		tracing.RecordError(span, err, "failed to update release")
		return fmt.Errorf("failed to set release status to 'running': %w", err)
	}

	var err error
	if md.restartOnly {
		err = md.restartMachinesApp(ctx)
	} else {
		err = md.deployMachinesApp(ctx)
	}

	var status string
	switch {
	case err == nil:
		status = "complete"
	case errors.Is(err, context.Canceled):
		// Provide an extra second to try to update the release status.
		status = "interrupted"
		var cancel func()
		ctx, cancel = context.WithTimeout(onInterruptContext, time.Second)
		defer cancel()
	default:
		status = "failed"
	}

	if updateErr := md.updateReleaseInBackend(ctx, status); updateErr != nil {
		if err == nil {
			err = fmt.Errorf("failed to set final release status: %w", updateErr)
		} else {
			terminal.Warnf("failed to set final release status after deployment failure: %v\n", updateErr)
		}
	}

	// no need to run dns checks if the deployment failed
	if !md.skipDNSChecks && err == nil {
		if err := md.checkDNS(ctx); err != nil {
			terminal.Warnf("DNS checks failed: %v\n", err)
		}
	}

	if err != nil {
		tracing.RecordError(span, err, "failed to deploy machines")
	}
	return err
}

// restartMachinesApp only restarts existing machines but updates their release metadata
func (md *machineDeployment) restartMachinesApp(ctx context.Context) error {
	ctx, span := tracing.GetTracer().Start(ctx, "restart_machines")
	defer span.End()

	if err := md.machineSet.AcquireLeases(ctx, md.leaseTimeout); err != nil {
		tracing.RecordError(span, err, "failed to acquire lease")
		return err
	}
	defer md.machineSet.ReleaseLeases(ctx) // skipcq: GO-S2307
	md.machineSet.StartBackgroundLeaseRefresh(ctx, md.leaseTimeout, md.leaseDelayBetween)

	machineUpdateEntries := lo.Map(md.machineSet.GetMachines(), func(lm machine.LeasableMachine, _ int) *machineUpdateEntry {
		return &machineUpdateEntry{leasableMachine: lm, launchInput: md.launchInputForRestart(lm.Machine())}
	})

	return md.updateExistingMachines(ctx, machineUpdateEntries)
}

func (md *machineDeployment) inferCanaryGuest(processGroup string) *fly.MachineGuest {
	canaryGuest := md.machineGuest
	for _, lm := range md.machineSet.GetMachines() {
		machine := lm.Machine()
		machineGuest := machine.Config.Guest
		switch {
		case machine.ProcessGroup() != processGroup:
			continue
		case machineGuest == nil: // shouldn't be possible but just in case
			continue
		case machineGuest.CPUKind != canaryGuest.CPUKind,
			machineGuest.CPUs != canaryGuest.CPUs,
			machineGuest.MemoryMB != canaryGuest.MemoryMB:
			return machineGuest
		}
	}

	return canaryGuest
}

func (md *machineDeployment) deployCanaryMachines(ctx context.Context) (err error) {
	ctx, span := tracing.GetTracer().Start(ctx, "deploy_canary")
	defer span.End()

	groupsInConfig := md.ProcessNames()
	total := len(groupsInConfig)
	sl := statuslogger.Create(ctx, total, true)
	defer sl.Destroy(false)

	errors, ctx := errgroup.WithContext(ctx)

	for idx, name := range groupsInConfig {
		ctx := statuslogger.NewContext(ctx, sl.Line(idx))
		statuslogger.LogfStatus(ctx,
			statuslogger.StatusRunning,
			"Creating canary machine for group %s",
			md.colorize.Bold(name),
		)

		// variable name shadowing to make go-vet happy
		name := name
		errors.Go(func() error {
			var err error
			lm, err := md.spawnMachineInGroup(ctx, name, nil,
				withMeta(metadata{key: "fly_canary", value: "true"}),
				withGuest(md.inferCanaryGuest(name)),
				withDns(&fly.DNSConfig{SkipRegistration: true}),
			)
			if err != nil {
				tracing.RecordError(span, err, "failed to provision canary machine")
				firstLine, _, _ := strings.Cut(err.Error(), "\n")
				statuslogger.LogfStatus(ctx, statuslogger.StatusFailure, "Failed to create canary machine: %s", firstLine)
				return err
			}

			defer func() {
				if err == nil {
					if destroyErr := machcmd.Destroy(ctx, md.app, lm.Machine(), true); destroyErr != nil {
						err = destroyErr
					}
				}
			}()

			if err = md.runTestMachines(ctx, lm.Machine()); err != nil {
				tracing.RecordError(span, err, "failed to run test machine for canary machine")
				firstLine, _, _ := strings.Cut(err.Error(), "\n")
				statuslogger.LogfStatus(ctx, statuslogger.StatusFailure, "Failed to run test machine for canary machine: %s", firstLine)
				return err
			}

			return err
		})
	}

	if err := errors.Wait(); err != nil {
		return err
	}

	return nil
}

// Create machines for new process groups
func (md *machineDeployment) deployCreateMachinesForGroups(ctx context.Context, processGroupMachineDiff ProcessGroupsDiff) (err error) {
	groupsWithAutostopEnabled := make(map[string]bool)
	groups := maps.Keys(processGroupMachineDiff.groupsNeedingMachines)
	total := len(groups)
	slices.Sort(groups)

	sl := statuslogger.Create(ctx, total, true)
	defer sl.Destroy(false)

	for idx, name := range groups {
		ctx := statuslogger.NewContext(ctx, sl.Line(idx))
		statuslogger.LogStatus(ctx, statuslogger.StatusRunning, "Launching new machine")
		fmt.Fprintf(md.io.Out, "No machines in group %s, launching a new machine\n", md.colorize.Bold(name))
		leasableMachine, err := md.spawnMachineInGroup(ctx, name, nil)
		if err != nil {
			statuslogger.Failed(ctx, err)
			return err
		}

		groupConfig, err := md.appConfig.Flatten(name)
		if err != nil {
			statuslogger.Failed(ctx, err)
			return err
		}

		services := groupConfig.AllServices()
		for _, s := range services {
			if s.AutoStopMachines != nil && *s.AutoStopMachines {
				groupsWithAutostopEnabled[name] = true
			}
		}

		// Create spare machines that increases availability unless --ha=false was used
		if !md.increasedAvailability {
			continue
		}

		// TODO(Ali): This overwrites the main machine's status log with the standby machine's status log.

		// We strive to provide a HA setup according to:
		// - Create only 1 machine if the group has mounts
		// - Create 2 machines for groups with services
		// - Create 1 always-on and 1 standby machine for groups without services
		switch {
		case len(groupConfig.Mounts) > 0:
			continue
		case len(services) > 0:
			fmt.Fprintf(md.io.Out, "Creating a second machine to increase service availability\n")
			if _, err := md.spawnMachineInGroup(ctx, name, nil); err != nil {
				statuslogger.Failed(ctx, err)
				return err
			}
		default:
			fmt.Fprintf(md.io.Out, "Creating a standby machine for %s\n", md.colorize.Bold(leasableMachine.Machine().ID))
			standbyFor := []string{leasableMachine.Machine().ID}
			if _, err := md.spawnMachineInGroup(ctx, name, standbyFor); err != nil {
				statuslogger.Failed(ctx, err)
				return err
			}
		}
	}

	if total > 0 {
		fmt.Fprintf(md.io.ErrOut, "Finished launching new machines\n")
	}

	if len(groupsWithAutostopEnabled) > 0 {
		groupNames := lo.Keys(groupsWithAutostopEnabled)
		slices.Sort(groupNames)
		fmt.Fprintf(md.io.Out,
			"\n%s The machines for [%s] have services with 'auto_stop_machines = true' that will be stopped when idling\n\n",
			md.colorize.Yellow("NOTE:"),
			md.colorize.Bold(strings.Join(groupNames, ",")),
		)
	}
	return nil
}

// deployMachinesApp executes the following flow:
//   - Run release command
//   - Remove spare machines from removed groups
//   - Launch new machines on new groups
//   - Update existing machines
func (md *machineDeployment) deployMachinesApp(ctx context.Context) error {
	ctx, span := tracing.GetTracer().Start(ctx, "deploy_new_machines")
	defer span.End()

	if !md.skipReleaseCommand {
		if err := md.runReleaseCommand(ctx); err != nil {
			return fmt.Errorf("release command failed - aborting deployment. %w", err)
		}
	}

	if err := md.machineSet.AcquireLeases(ctx, md.leaseTimeout); err != nil {
		return err
	}
	defer md.machineSet.ReleaseLeases(ctx) // skipcq: GO-S2307
	md.machineSet.StartBackgroundLeaseRefresh(ctx, md.leaseTimeout, md.leaseDelayBetween)

	processGroupMachineDiff := md.resolveProcessGroupChanges()
	md.warnAboutProcessGroupChanges(processGroupMachineDiff)

	if md.strategy == "canary" && !md.isFirstDeploy {
		if err := md.deployCanaryMachines(ctx); err != nil {
			return err
		}
	}

	// Destroy machines that don't fit the current process groups
	if err := md.machineSet.RemoveMachines(ctx, processGroupMachineDiff.machinesToRemove); err != nil {
		return err
	}
	for _, mach := range processGroupMachineDiff.machinesToRemove {
		if err := machcmd.Destroy(ctx, md.app, mach.Machine(), true); err != nil {
			return err
		}
	}

	if !md.updateOnly {
		if err := md.deployCreateMachinesForGroups(ctx, processGroupMachineDiff); err != nil {
			return err
		}
	}

	var machineUpdateEntries []*machineUpdateEntry
	for _, lm := range md.machineSet.GetMachines() {
		li, err := md.launchInputForUpdate(lm.Machine())
		if err != nil {
			return fmt.Errorf("failed to update machine configuration for %s: %w", lm.FormattedMachineId(), err)
		}
		machineUpdateEntries = append(machineUpdateEntries, &machineUpdateEntry{leasableMachine: lm, launchInput: li})
	}

	return md.updateExistingMachines(ctx, machineUpdateEntries)
}

type machineUpdateEntry struct {
	leasableMachine machine.LeasableMachine
	launchInput     *fly.LaunchMachineInput
}

type machineUpdateEntries []*machineUpdateEntry

func (m machineUpdateEntries) machines() []machine.LeasableMachine {
	return lo.Map(m, func(i *machineUpdateEntry, _ int) machine.LeasableMachine { return i.leasableMachine })
}

func errorIsTimeout(err error) bool {
	// Match an error against various known timeout conditions.
	// This is probably a sign that we need to standardize this better, but it works for now.

	var timeoutErr machine.WaitTimeoutErr
	if errors.As(err, &timeoutErr) {
		return true
	}

	if strings.Contains(err.Error(), "net/http: request canceled") {
		return true
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	if errors.Is(err, ErrWaitTimeout) {
		return true
	}

	return false
}

// suggestChangeWaitTimeout appends a suggestion to change the specified flag name
// if and only if the error is caused by a timeout.
// If the error is not a timeout, it's returned unchanged.
func suggestChangeWaitTimeout(err error, flagName string) error {
	if errorIsTimeout(err) {

		suggestIncreaseTimeout := fmt.Sprintf("increasing the timeout with the --%s flag", flagName)

		descript := ""
		suggest := ""

		// Both of these branches give the suggestion to change the timeout,
		// but we only suggest changing region on machine start.

		var timeoutErr machine.WaitTimeoutErr
		if errors.As(err, &timeoutErr) && timeoutErr.DesiredState() == fly.MachineStateStarted {
			// If we timed out waiting for a machine to start, we want to suggest that there could be a region issue preventing
			// the machine from finishing its state transition. (e.g. slow image pulls, volume trouble, etc.)
			descript = "Your machine was created, but never started. This could mean that your app is taking a long time to start,\nbut it could be indicative of a region issue."
			suggest = fmt.Sprintf("You can try deploying to a different region,\nor you can try %s", suggestIncreaseTimeout)
		} else {
			// If we timed out waiting for a different state, we want to suggest that the timeout could be too short.
			// You can't really suggest changing regions in cases where you're not starting machines, so this is the
			// best advice we can give.
			descript = fmt.Sprintf("Your machine never reached the state \"%s\".", timeoutErr.DesiredState())
			suggest = fmt.Sprintf("You can try %s", suggestIncreaseTimeout)
		}

		err = flyerr.GenericErr{
			Err:      err.Error(),
			Descript: descript,
			Suggest:  suggest,
		}
	}
	return err
}

func (md *machineDeployment) waitForMachine(ctx context.Context, e *machineUpdateEntry) error {
	lm := e.leasableMachine
	// Don't wait for SkipLaunch machines, they are updated but not started
	if e.launchInput.SkipLaunch {
		return nil
	}

	if !md.skipHealthChecks {
		if err := lm.WaitForState(ctx, fly.MachineStateStarted, md.waitTimeout, false); err != nil {
			err = suggestChangeWaitTimeout(err, "wait-timeout")
			return err
		}

		if err := md.runTestMachines(ctx, e.leasableMachine.Machine()); err != nil {
			return err
		}
	}

	if err := md.doSmokeChecks(ctx, lm); err != nil {
		return err
	}

	if !md.skipHealthChecks {
		// FIXME: combine this wait with the wait for start as one update line (or two per in noninteractive case)
		if err := lm.WaitForHealthchecksToPass(ctx, md.waitTimeout); err != nil {
			md.warnAboutIncorrectListenAddress(ctx, lm)
			err = suggestChangeWaitTimeout(err, "wait-timeout")
			return err
		}
	}

	md.warnAboutIncorrectListenAddress(ctx, lm)
	return nil
}

func (md *machineDeployment) updateExistingMachines(ctx context.Context, updateEntries []*machineUpdateEntry) (err error) {
	ctx, span := tracing.GetTracer().Start(ctx, "update_machines", trace.WithAttributes(
		attribute.String("strategy", md.strategy),
	))
	defer func() {
		if err != nil {
			tracing.RecordError(span, err, "update failed")
		}
		span.End()
	}()

	if len(updateEntries) == 0 {
		return nil
	}

	fmt.Fprintf(md.io.Out, "Updating existing machines in '%s' with %s strategy\n", md.colorize.Bold(md.app.Name), md.strategy)

	switch md.strategy {
	case "bluegreen":
		// TODO(billy) do machine checks here
		return md.updateUsingBlueGreenStrategy(ctx, updateEntries)
	case "immediate":
		return md.updateUsingImmediateStrategy(ctx, updateEntries)
	case "canary", "rolling":
		fallthrough
	default:
		return md.updateUsingRollingStrategy(ctx, updateEntries)
	}
}

func (md *machineDeployment) updateUsingBlueGreenStrategy(ctx context.Context, updateEntries []*machineUpdateEntry) error {
	bg := BlueGreenStrategy(md, updateEntries)
	if err := bg.Deploy(ctx); err != nil {
		if rollbackErr := bg.Rollback(ctx, err); rollbackErr != nil {
			fmt.Fprintf(md.io.ErrOut, "Error in rollback: %s\n", rollbackErr)
			fmt.Fprintf(md.io.ErrOut, "Deployment failed after error: %s\n", err)
			return rollbackErr
		}

		fmt.Fprintf(md.io.ErrOut, "Deployment failed after error: %s\n", err)
		return suggestChangeWaitTimeout(err, "wait-timeout")
	}
	return nil
}

func (md *machineDeployment) updateUsingImmediateStrategy(parentCtx context.Context, updateEntries []*machineUpdateEntry) error {
	parentCtx, span := tracing.GetTracer().Start(parentCtx, "immediate")
	defer span.End()

	sl := statuslogger.Create(parentCtx, len(updateEntries), true)
	defer sl.Destroy(false)

	updatesPool := pool.New().WithErrors().WithContext(parentCtx)
	if md.maxConcurrent > 0 {
		updatesPool = updatesPool.WithMaxGoroutines(md.maxConcurrent)
	}

	for i, e := range updateEntries {
		e := e
		eCtx := statuslogger.NewContext(parentCtx, sl.Line(i))
		fmtID := e.leasableMachine.FormattedMachineId()
		statusRunning := func() {
			statuslogger.LogfStatus(eCtx,
				statuslogger.StatusRunning,
				"Updating %s",
				md.colorize.Bold(fmtID),
			)
		}
		statusFailure := func(err error) {
			if errors.Is(err, context.Canceled) {
				statuslogger.LogfStatus(eCtx,
					statuslogger.StatusFailure,
					"Machine %s update %s",
					md.colorize.Bold(fmtID),
					md.colorize.Red("canceled while it was in progress"),
				)
			} else {
				statuslogger.LogfStatus(eCtx,
					statuslogger.StatusFailure,
					"Machine %s update %s: %s",
					md.colorize.Bold(fmtID),
					md.colorize.Red("failed"),
					err.Error(),
				)
			}
		}
		statusSuccess := func() {
			statuslogger.LogfStatus(eCtx,
				statuslogger.StatusSuccess,
				"Machine %s update %s",
				md.colorize.Bold(fmtID),
				md.colorize.Green("succeeded"),
			)
		}

		updatesPool.Go(func(_ context.Context) error {
			statusRunning()
			if err := md.updateMachine(eCtx, e); err != nil {
				tracing.RecordError(span, err, "failed to update machine")
				statusFailure(err)
				return err
			}
			statusSuccess()
			return nil
		})
	}

	return updatesPool.Wait()
}

func (md *machineDeployment) updateUsingRollingStrategy(parentCtx context.Context, updateEntries []*machineUpdateEntry) error {
	parentCtx, span := tracing.GetTracer().Start(parentCtx, "rolling", trace.WithAttributes(attribute.String("strategy", md.strategy)))
	defer span.End()

	sl := statuslogger.Create(parentCtx, len(updateEntries), true)
	defer sl.Destroy(false)

	// Rolling strategy
	slices.SortFunc(updateEntries, func(a, b *machineUpdateEntry) int {
		return cmp.Compare(a.leasableMachine.Machine().ID, b.leasableMachine.Machine().ID)
	})

	// Group updates by process group
	entriesByGroup := lo.GroupBy(updateEntries, func(e *machineUpdateEntry) string {
		return e.launchInput.Config.ProcessGroup()
	})

	startIdx := 0
	groupsPool := pool.New().
		WithErrors().
		WithMaxGoroutines(rollingStrategyMaxConcurrentGroups).
		WithContext(parentCtx).
		WithCancelOnError()

	for group, entries := range entriesByGroup {
		entries := entries
		startIdx += len(entries)
		groupsPool.Go(func(ctx context.Context) error {
			return md.updateEntriesGroup(ctx, group, entries, sl, startIdx-len(entries))
		})
	}

	return groupsPool.Wait()
}

func (md *machineDeployment) updateEntriesGroup(parentCtx context.Context, group string, entries []*machineUpdateEntry, sl statuslogger.StatusLogger, startIdx int) error {
	parentCtx, span := tracing.GetTracer().Start(parentCtx, "update_entries_in_group", trace.WithAttributes(
		attribute.Int("start_id", startIdx),
		attribute.String("group", group),
		attribute.Int("max_unavailable", int(md.maxUnavailable)),
	))
	defer span.End()

	var poolSize int
	switch mu := md.maxUnavailable; {
	case mu >= 1:
		poolSize = int(mu)
	case mu > 0:
		poolSize = int(math.Ceil(float64(len(entries)) * mu))
	default:
		return fmt.Errorf("Invalid --max-unavailable value: %v", mu)
	}

	span.SetAttributes(attribute.Int("pool_size", poolSize))

	updatePool := pool.New().
		WithErrors().
		WithMaxGoroutines(poolSize).
		WithContext(parentCtx).
		WithCancelOnError()

	for idx, e := range entries {
		e := e
		eCtx := statuslogger.NewContext(parentCtx, sl.Line(startIdx+idx))
		fmtID := e.leasableMachine.FormattedMachineId()

		statusRunning := func() {
			statuslogger.LogfStatus(eCtx,
				statuslogger.StatusRunning,
				"Updating %s",
				md.colorize.Bold(fmtID),
			)
		}
		statusFailure := func(err error) {
			if errors.Is(err, context.Canceled) {
				statuslogger.LogfStatus(eCtx,
					statuslogger.StatusFailure,
					"Machine %s update %s",
					md.colorize.Bold(fmtID),
					md.colorize.Red("canceled while it was in progress"),
				)
			} else {
				statuslogger.LogfStatus(eCtx,
					statuslogger.StatusFailure,
					"Machine %s update %s: %s",
					md.colorize.Bold(fmtID),
					md.colorize.Red("failed"),
					err.Error(),
				)
			}
		}
		statusSkipped := func() {
			statuslogger.LogfStatus(eCtx,
				statuslogger.StatusFailure,
				"Machine %s update %s",
				md.colorize.Bold(fmtID),
				md.colorize.Yellow("canceled"),
			)
		}
		statusSuccess := func() {
			statuslogger.LogfStatus(eCtx,
				statuslogger.StatusSuccess,
				"Machine %s update %s",
				md.colorize.Bold(fmtID),
				md.colorize.Green("succeeded"),
			)
		}
		updateFunc := func(poolCtx context.Context) error {
			ctx, span := tracing.GetTracer().Start(eCtx, "update", trace.WithAttributes(
				attribute.Int("id", idx),
			))
			defer span.End()

			// If the pool context is done, it means some other machine update failed
			select {
			case <-poolCtx.Done():
				statusSkipped()
				return poolCtx.Err()
			default:
				statusRunning()
			}

			if err := md.updateMachine(ctx, e); err != nil {
				statusFailure(err)
				tracing.RecordError(span, err, "failed to update machine")
				return err
			}
			if err := md.waitForMachine(ctx, e); err != nil {
				tracing.RecordError(span, err, "failed to wait for machine")
				statusFailure(err)
				return err
			}

			statusSuccess()
			return nil
		}

		// Slow start by updating one machine and then the rest in groups if the spearhead succeeded
		if idx == 0 {
			err := updateFunc(parentCtx)
			if err != nil {
				return err
			}
		} else {
			updatePool.Go(updateFunc)
		}
	}

	return updatePool.Wait()
}

func (md *machineDeployment) updateMachine(ctx context.Context, e *machineUpdateEntry) error {
	ctx, span := tracing.GetTracer().Start(ctx, "update_machine", trace.WithAttributes(
		attribute.String("id", e.launchInput.ID),
		attribute.Bool("requires_replacement", e.launchInput.RequiresReplacement),
	))
	defer span.End()

	fmtID := e.leasableMachine.FormattedMachineId()

	replaceMachine := func() error {
		statuslogger.Logf(ctx, "Replacing %s by new machine", md.colorize.Bold(fmtID))
		if err := md.updateMachineByReplace(ctx, e); err != nil {
			return err
		}
		statuslogger.Logf(ctx, "Created machine %s", md.colorize.Bold(fmtID))
		return nil
	}

	if e.launchInput.RequiresReplacement {
		return replaceMachine()
	}

	statuslogger.Logf(ctx, "Updating %s", md.colorize.Bold(fmtID))
	if err := md.updateMachineInPlace(ctx, e); err != nil {
		switch {
		case len(e.leasableMachine.Machine().Config.Mounts) > 0:
			// Replacing a machine with a volume will cause the placement logic to pick wthe same host
			// dismissing the value of replacing it in case of lack of host capacity
			return err
		case strings.Contains(err.Error(), "could not reserve resource for machine"):
			return replaceMachine()
		default:
			return err
		}
	}
	return nil
}

func (md *machineDeployment) updateMachineByReplace(ctx context.Context, e *machineUpdateEntry) error {
	ctx, span := tracing.GetTracer().Start(ctx, "update_by_replace", trace.WithAttributes(attribute.String("id", e.launchInput.ID)))
	defer span.End()

	lm := e.leasableMachine
	// If machine requires replacement, destroy old machine and launch a new one
	// This can be the case for machines that changes its volumes.
	if err := lm.Destroy(ctx, true); err != nil {
		return err
	}

	// Acquire a lease on the new machine to ensure external factors can't stop or update it
	// while we wait for its state and/or health checks
	e.launchInput.LeaseTTL = int(md.waitTimeout.Seconds())

	newMachineRaw, err := md.flapsClient.Launch(ctx, *e.launchInput)
	if err != nil {
		return err
	}

	lm = machine.NewLeasableMachine(md.flapsClient, md.io, newMachineRaw)
	defer lm.ReleaseLease(ctx)
	e.leasableMachine = lm
	return nil
}

func (md *machineDeployment) updateMachineInPlace(ctx context.Context, e *machineUpdateEntry) error {
	lm := e.leasableMachine
	return lm.Update(ctx, *e.launchInput)
}

type spawnOptions struct {
	meta  []metadata
	guest *fly.MachineGuest
	dns   *fly.DNSConfig
}

type spawnOptionsFn func(*spawnOptions)

func withMeta(m metadata) spawnOptionsFn {
	return func(o *spawnOptions) {
		o.meta = append(o.meta, m)
	}
}

func withGuest(guest *fly.MachineGuest) spawnOptionsFn {
	return func(o *spawnOptions) {
		o.guest = guest
	}
}

func withDns(dns *fly.DNSConfig) spawnOptionsFn {
	return func(o *spawnOptions) {
		o.dns = dns
	}
}

type metadata struct {
	key   string
	value string
}

func (md *machineDeployment) spawnMachineInGroup(ctx context.Context, groupName string, standbyFor []string, opts ...spawnOptionsFn) (machine.LeasableMachine, error) {
	options := spawnOptions{
		meta:  []metadata{},
		guest: md.machineGuest,
	}
	for _, opt := range opts {
		opt(&options)
	}

	launchInput, err := md.launchInputForLaunch(groupName, options.guest, standbyFor)
	if err != nil {
		return nil, fmt.Errorf("error creating machine configuration: %w", err)
	}

	for _, m := range options.meta {
		launchInput.Config.Metadata[m.key] = m.value
	}

	if options.dns != nil {
		launchInput.Config.DNS = options.dns
	}

	// Acquire a lease on the new machine to ensure external factors can't stop or update it
	// while we wait for its state and/or health checks
	launchInput.LeaseTTL = int(md.waitTimeout.Seconds())

	newMachineRaw, err := md.flapsClient.Launch(ctx, *launchInput)
	if err != nil {
		relCmdWarning := ""
		if strings.Contains(err.Error(), "please add a payment method") && !md.releaseCommandMachine.IsEmpty() {
			relCmdWarning = "\nPlease note that release commands run in their own ephemeral machine, and therefore count towards the machine limit."
		}
		return nil, fmt.Errorf("error creating a new machine: %w%s", err, relCmdWarning)
	}

	lm := machine.NewLeasableMachine(md.flapsClient, md.io, newMachineRaw)
	statuslogger.Logf(ctx, "Machine %s was created", md.colorize.Bold(lm.FormattedMachineId()))
	defer lm.ReleaseLease(ctx)

	// Don't wait for SkipLaunch machines, they are created but not started
	if launchInput.SkipLaunch {
		return lm, nil
	}

	// Roll up as fast as possible when using immediate strategy
	if md.strategy == "immediate" {
		return lm, nil
	}

	// Otherwise wait for the machine to start
	if err := md.doSmokeChecks(ctx, lm); err != nil {
		return nil, err
	}

	// And wait (or not) for successful health checks
	if !md.skipHealthChecks {
		// Don't wait for state if the --detach flag isn't specified
		if err := lm.WaitForState(ctx, fly.MachineStateStarted, md.waitTimeout, false); err != nil {
			err = suggestChangeWaitTimeout(err, "wait-timeout")
			return nil, err
		}

		if err := lm.WaitForHealthchecksToPass(ctx, md.waitTimeout); err != nil {
			md.warnAboutIncorrectListenAddress(ctx, lm)
			err = suggestChangeWaitTimeout(err, "wait-timeout")
			return nil, err
		}

		statuslogger.LogfStatus(ctx,
			statuslogger.StatusSuccess,
			"Machine %s update finished: %s",
			md.colorize.Bold(lm.FormattedMachineId()),
			md.colorize.Green("success"),
		)
	}

	md.warnAboutIncorrectListenAddress(ctx, lm)

	return lm, nil
}

func (md *machineDeployment) resolveProcessGroupChanges() ProcessGroupsDiff {
	output := ProcessGroupsDiff{
		groupsToRemove:        map[string]int{},
		groupsNeedingMachines: map[string]bool{},
	}

	groupsInConfig := md.ProcessNames()
	groupHasMachine := map[string]bool{}

	for _, leasableMachine := range md.machineSet.GetMachines() {
		name := leasableMachine.Machine().ProcessGroup()
		if slices.Contains(groupsInConfig, name) {
			groupHasMachine[name] = true
		} else {
			output.groupsToRemove[name] += 1
			output.machinesToRemove = append(output.machinesToRemove, leasableMachine)
		}
	}

	for _, name := range groupsInConfig {
		if ok := groupHasMachine[name]; !ok {
			output.groupsNeedingMachines[name] = true
		}
	}

	return output
}

func (md *machineDeployment) warnAboutProcessGroupChanges(diff ProcessGroupsDiff) {
	willAddMachines := len(diff.groupsNeedingMachines) != 0
	willRemoveMachines := diff.machinesToRemove != nil

	if !willAddMachines && !willRemoveMachines {
		return
	}

	if md.isFirstDeploy {
		fmt.Fprintln(md.io.Out, "This deployment will:")
	} else {
		fmt.Fprintln(md.io.Out, "Process groups have changed. This will:")
	}

	if willRemoveMachines {
		bullet := md.colorize.Red("*")
		for grp, numMach := range diff.groupsToRemove {
			pluralS := lo.Ternary(numMach == 1, "", "s")
			fmt.Fprintf(md.io.Out, " %s destroy %d \"%s\" machine%s\n", bullet, numMach, grp, pluralS)
		}
	}
	if willAddMachines {
		bullet := md.colorize.Green("*")
		for name := range diff.groupsNeedingMachines {
			var description string
			groupConfig, err := md.appConfig.Flatten(name)
			switch {
			case err != nil:
				continue
			case !md.increasedAvailability || len(groupConfig.Mounts) > 0:
				description = fmt.Sprintf("1 \"%s\" machine", name)
			case len(groupConfig.AllServices()) > 0:
				description = fmt.Sprintf("2 \"%s\" machines", name)
			default:
				description = fmt.Sprintf("1 \"%s\" machine and 1 standby machine for it", name)
			}
			fmt.Fprintf(md.io.Out, " %s create %s\n", bullet, description)
		}
	}
	fmt.Fprint(md.io.Out, "\n")
}

func (md *machineDeployment) warnAboutIncorrectListenAddress(ctx context.Context, lm machine.LeasableMachine) {
	group := lm.Machine().ProcessGroup()

	if _, seen := md.listenAddressChecked.LoadOrStore(group, struct{}{}); seen {
		return
	}

	groupConfig, err := md.appConfig.Flatten(group)
	if err != nil {
		return
	}
	services := groupConfig.AllServices()

	tcpServices := make(map[int]struct{})
	for _, s := range services {
		if s.Protocol == "tcp" {
			tcpServices[s.InternalPort] = struct{}{}
		}
	}

	processes, err := md.flapsClient.GetProcesses(ctx, lm.Machine().ID)
	// Let's not fail the whole deployment because of this, as listen address check is just a warning
	if err != nil {
		return
	}

	var foundSockets int
	for _, proc := range processes {
		for _, ls := range proc.ListenSockets {
			foundSockets += 1

			host, portStr, err := net.SplitHostPort(ls.Address)
			if err != nil {
				continue
			}
			port, err := strconv.Atoi(portStr)
			if err != nil {
				continue
			}

			ip := net.ParseIP(host)

			// We don't know VM's internal ipv4 which is also a valid address to bind to.
			// Let's assume that whoever binds to a non-loopback address knows what they are doing.
			// If we expose this address to flyctl later, we can revisit this logic.
			if !ip.IsLoopback() {
				delete(tcpServices, port)
			}
		}
	}

	// This can either mean that nothing is listening or that VM is running old init that doesn't expose
	// listen sockets. Until we have a way to update init on already created VMs let's ignore this
	// and pretend that this is old init.
	if foundSockets == 0 {
		return
	}

	// All services are covered
	if len(tcpServices) == 0 {
		return
	}

	fmt.Fprintf(md.io.ErrOut, "\n%s The app is not listening on the expected address and will not be reachable by fly-proxy.\n", md.colorize.Yellow("WARNING"))
	fmt.Fprintf(md.io.ErrOut, "You can fix this by configuring your app to listen on the following addresses:\n")
	for port := range tcpServices {
		fmt.Fprintf(md.io.ErrOut, "  - %s\n", md.colorize.Green("0.0.0.0:"+strconv.Itoa(port)))
	}
	fmt.Fprintf(md.io.ErrOut, "Found these processes inside the machine with open listening sockets:\n")

	table := helpers.MakeSimpleTable(md.io.ErrOut, []string{"Process", "Addresses"})
	for _, proc := range processes {
		var addresses []string
		for _, ls := range proc.ListenSockets {
			if ls.Proto == "tcp" {
				addresses = append(addresses, ls.Address)
			}
		}
		if len(addresses) > 0 {
			table.Append([]string{proc.Command, strings.Join(addresses, ", ")})
		}
	}
	table.Render()
	fmt.Fprintf(md.io.ErrOut, "\n")
}

func (md *machineDeployment) doSmokeChecks(ctx context.Context, lm machine.LeasableMachine) (err error) {
	if md.skipSmokeChecks {
		return nil
	}

	if err = lm.WaitForSmokeChecksToPass(ctx); err == nil {
		return nil
	}

	resumeLogFn := statuslogger.Pause(ctx)
	defer resumeLogFn()

	fmt.Fprintf(md.io.ErrOut, "Smoke checks for %s failed: %v\n", md.colorize.Bold(lm.Machine().ID), err)
	fmt.Fprintf(md.io.ErrOut, "Check its logs: here's the last lines below, or run 'fly logs -i %s':\n", lm.Machine().ID)
	logs, _, logErr := md.apiClient.GetAppLogs(ctx, md.app.Name, "", md.appConfig.PrimaryRegion, lm.Machine().ID)
	if fly.IsNotAuthenticatedError(logErr) {
		fmt.Fprintf(md.io.ErrOut, "Warn: not authorized to retrieve app logs (this can happen when using deploy tokens), so we can't show you what failed. Use `fly logs -i %s` or open the monitoring dashboard to see them: https://fly.io/apps/%s/monitoring?region=&instance=%s\n", lm.Machine().ID, md.appConfig.AppName, lm.Machine().ID)
	} else {
		if logErr != nil {
			return fmt.Errorf("error getting logs for machine %s: %w", lm.Machine().ID, logErr)
		}
		for _, l := range logs {
			// Ideally we should use InstanceID here, but it's not available in the logs.
			if l.Timestamp >= lm.Machine().UpdatedAt {
				fmt.Fprintf(md.io.ErrOut, "  %s\n", l.Message)
			}
		}
	}

	return fmt.Errorf("smoke checks for %s failed: %v", lm.Machine().ID, err)
}

func (md *machineDeployment) checkDNS(ctx context.Context) error {
	ctx, span := tracing.GetTracer().Start(ctx, "check_dns")
	defer span.End()
	ctx, cancel := context.WithTimeout(ctx, time.Second*70)
	defer cancel()

	client := flyutil.ClientFromContext(ctx)
	ipAddrs, err := client.GetIPAddresses(ctx, md.appConfig.AppName)
	if err != nil {
		tracing.RecordError(span, err, "failed to get ip addresses")
		return err
	}

	if appURL := md.appConfig.URL(); appURL != nil && len(ipAddrs) > 0 {
		iostreams := iostreams.FromContext(ctx)
		fmt.Fprintf(iostreams.ErrOut, "Checking DNS configuration for %s\n", md.colorize.Bold(appURL.Host))

		fqdn := dns.Fqdn(appURL.Host)
		c := dns.Client{
			Dialer:       &net.Dialer{Timeout: time.Minute},
			Timeout:      time.Minute,
			DialTimeout:  time.Minute,
			ReadTimeout:  time.Minute,
			WriteTimeout: time.Minute,
		}

		b := backoff.NewExponentialBackOff()
		b.InitialInterval = 1 * time.Second
		b.MaxInterval = 5 * time.Second
		b.MaxElapsedTime = 60 * time.Second

		return backoff.Retry(func() error {
			m := new(dns.Msg)

			var numIPv4, numIPv6 int
			for _, ipAddr := range ipAddrs {
				if ipAddr.Type == "v4" || ipAddr.Type == "shared_v4" {
					numIPv4 += 1
				} else if ipAddr.Type == "v6" {
					numIPv6 += 1
				}
			}

			span.SetAttributes(attribute.Int("v4_count", numIPv4))
			span.SetAttributes(attribute.Int("v6_count", numIPv6))

			m.SetQuestion(fqdn, dns.TypeA)
			span.SetAttributes(attribute.String("v4_question", m.String()))
			answerv4, _, err := c.Exchange(m, "8.8.8.8:53")
			if err != nil {
				tracing.RecordError(span, err, "failed to exchange v4")
				return err
			} else if len(answerv4.Answer) != numIPv4 {
				span.SetAttributes(attribute.String("v4_answer", answerv4.String()))
				tracing.RecordError(span, errors.New("v4 response count mismatch"), "v4 response count mismatch")
				return fmt.Errorf("expected %d A records for %s, got %d", numIPv4, fqdn, len(answerv4.Answer))
			}

			m.SetQuestion(fqdn, dns.TypeAAAA)
			span.SetAttributes(attribute.String("v6_question", m.String()))
			answerv6, _, err := c.Exchange(m, "8.8.8.8:53")
			if err != nil {
				tracing.RecordError(span, err, "failed to exchange v4")
				return err
			} else if len(answerv6.Answer) != numIPv6 {
				span.SetAttributes(attribute.String("v6_answer", answerv6.String()))
				tracing.RecordError(span, errors.New("v6 response count mismatch"), "v6 response count mismatch")
				return fmt.Errorf("expected %d AAAA records for %s, got %d", numIPv6, fqdn, len(answerv6.Answer))
			}

			return nil
		}, backoff.WithContext(b, ctx))

	} else {
		return nil
	}
}
