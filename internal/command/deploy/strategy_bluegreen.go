package deploy

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"errors"

	"github.com/avast/retry-go/v4"
	"github.com/hashicorp/go-multierror"
	"github.com/sourcegraph/conc/pool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/ctrlc"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
)

// TODO(ali): Use statuslogger here

var (
	ErrTagForDeletion        = errors.New("failed to mark as safe for deletion")
	ErrAborted               = errors.New("deployment aborted by user")
	ErrWaitTimeout           = errors.New("wait timeout")
	ErrCreateGreenMachine    = errors.New("failed to create green machines")
	ErrWaitForStartedState   = errors.New("could not get all green machines into started state")
	ErrWaitForHealthy        = errors.New("could not get all green machines to be healthy")
	ErrMarkReadyForTraffic   = errors.New("failed to mark green machines as ready")
	ErrCordonBlueMachines    = errors.New("failed to cordon blue machines")
	ErrStopBlueMachines      = errors.New("failed to stop blue machines")
	ErrWaitForStoppedState   = errors.New("could not get all blue machines into stopped state")
	ErrDestroyBlueMachines   = errors.New("failed to destroy previous deployment")
	ErrValidationError       = errors.New("app not in valid state for bluegreen deployments")
	ErrOrgLimit              = errors.New("app can't undergo bluegreen deployment due to org limits")
	ErrMultipleImageVersions = errors.New("found multiple image versions")

	safeToDestroyValue = "safe_to_destroy"
)

type RollbackLog struct {
	// this ensures that user invoked aborts after green machines are healthy
	// doesn't cause the greeen machines to be removed. eg. if someone aborts after cordoning blue machines
	canDeleteGreenMachines bool
	disableRollback        bool
}

type blueGreenWebClient interface {
	CanPerformBluegreenDeployment(ctx context.Context, appName string) (bool, error)
}

type blueGreen struct {
	greenMachines       machineUpdateEntries
	blueMachines        machineUpdateEntries
	flaps               flapsutil.FlapsClient
	apiClient           blueGreenWebClient
	io                  *iostreams.IOStreams
	colorize            *iostreams.ColorScheme
	clearLinesAbove     func(count int)
	timeout             time.Duration
	stopSignal          string
	aborted             chan struct{}
	healthLock          sync.RWMutex
	stateLock           sync.RWMutex
	ctrlcHook           ctrlc.Handle
	appConfig           *appconfig.Config
	hangingBlueMachines []string
	timestamp           string
	maxConcurrent       int

	rollbackLog RollbackLog

	waitBeforeStop   time.Duration
	waitBeforeCordon time.Duration
}

func BlueGreenStrategy(md *machineDeployment, blueMachines []*machineUpdateEntry) *blueGreen {
	bg := &blueGreen{
		greenMachines:       machineUpdateEntries{},
		blueMachines:        blueMachines,
		flaps:               md.flapsClient,
		apiClient:           md.apiClient,
		appConfig:           md.appConfig,
		timeout:             md.waitTimeout,
		stopSignal:          md.stopSignal,
		io:                  md.io,
		colorize:            md.colorize,
		clearLinesAbove:     md.logClearLinesAbove,
		aborted:             make(chan struct{}),
		healthLock:          sync.RWMutex{},
		stateLock:           sync.RWMutex{},
		hangingBlueMachines: []string{},
		timestamp:           fmt.Sprintf("%d", time.Now().Unix()),
		maxConcurrent:       md.maxConcurrent,
		rollbackLog:         RollbackLog{canDeleteGreenMachines: true, disableRollback: false},
	}

	bg.initialize()

	return bg
}

func (bg *blueGreen) initialize() {
	// Hook into Ctrl+C so that we can rollback the deployment when it's aborted.
	ctrlc.ClearHandlers()
	bg.ctrlcHook = ctrlc.Hook(sync.OnceFunc(func() {
		close(bg.aborted)
	}))

	bg.waitBeforeStop = 10 * time.Second
	bg.waitBeforeCordon = 10 * time.Second
}

func (bg *blueGreen) isAborted() bool {
	select {
	case <-bg.aborted:
		return true
	default:
		return false
	}
}

func (bg *blueGreen) sleepAbortable(d time.Duration) bool {
	select {
	case <-time.After(d):
		return false
	case <-bg.aborted:
		return true
	}
}

func (bg *blueGreen) CreateGreenMachines(ctx context.Context) error {
	ctx, span := tracing.GetTracer().Start(ctx, "green_machines_create")
	defer span.End()

	// Limit launch concurrency to a third of the machines to launch.
	// It helps workaround a resource allocation race when multiple machines
	// are created at the same time.
	createConcurrency := int(math.Max(1, math.Min(
		math.Ceil(float64(len(bg.blueMachines))/3),
		float64(bg.maxConcurrent),
	)))

	var lock sync.Mutex
	p := pool.New().
		WithErrors().
		WithFirstError().
		WithMaxGoroutines(createConcurrency)
	for _, mach := range bg.blueMachines {
		mach := mach
		p.Go(func() error {
			if bg.isAborted() {
				return ErrAborted
			}

			launchInput := mach.launchInput
			launchInput.SkipServiceRegistration = true
			launchInput.Config.Metadata[fly.MachineConfigMetadataKeyFlyctlBGTag] = bg.timestamp

			newMachineRaw, err := bg.flaps.Launch(ctx, *launchInput)
			if err != nil {
				tracing.RecordError(span, err, "failed to launch machine")
				return err
			}

			greenMachine := machine.NewLeasableMachine(bg.flaps, bg.io, newMachineRaw, true)
			defer releaseLease(ctx, greenMachine)

			lock.Lock()
			defer lock.Unlock()

			bg.greenMachines = append(bg.greenMachines, &machineUpdateEntry{greenMachine, launchInput})

			fmt.Fprintf(bg.io.ErrOut, "  Created machine %s\n", bg.colorize.Bold(greenMachine.FormattedMachineId()))
			return nil
		})
	}

	if err := p.Wait(); err != nil {
		return err
	}

	return nil
}

func (bg *blueGreen) renderMachineStates(state map[string]string) func() {
	firstRun := true

	previousView := map[string]string{}

	return func() {
		currentView := map[string]string{}
		rows := []string{}
		bg.stateLock.RLock()
		for id, status := range state {
			currentView[id] = status
			rows = append(rows, fmt.Sprintf("  Machine %s - %s", bg.colorize.Bold(id), bg.colorize.Green(status)))
		}
		bg.stateLock.RUnlock()

		if !firstRun && bg.changeDetected(currentView, previousView) {
			bg.clearLinesAbove(len(rows))
		}

		sort.Strings(rows)

		if bg.changeDetected(currentView, previousView) {
			fmt.Fprintf(bg.io.ErrOut, "%s\n", strings.Join(rows, "\n"))
			previousView = currentView
		}

		firstRun = false
	}
}

func (bg *blueGreen) allMachinesStarted(stateMap map[string]string) bool {
	started := 0
	bg.stateLock.RLock()
	for _, v := range stateMap {
		if v == "started" {
			started += 1
		}
	}
	bg.stateLock.RUnlock()

	return started == len(stateMap)
}

func (bg *blueGreen) WaitForGreenMachinesToBeStarted(ctx context.Context) error {
	ctx, span := tracing.GetTracer().Start(ctx, "green_machines_start_wait")
	defer span.End()

	wait := time.NewTicker(bg.timeout)
	machineIDToState := map[string]string{}
	for _, gm := range bg.greenMachines.machines() {
		machineIDToState[gm.FormattedMachineId()] = "created"
	}

	render := bg.renderMachineStates(machineIDToState)
	errChan := make(chan error)

	for _, gm := range bg.greenMachines {
		id := gm.leasableMachine.FormattedMachineId()

		if gm.launchInput.SkipLaunch {
			machineIDToState[id] = "started"
			continue
		}

		go func(lm machine.LeasableMachine) {
			err := machine.WaitForStartOrStop(ctx, lm.Machine(), "start", bg.timeout)
			if err != nil {
				errChan <- err
				return
			}

			bg.stateLock.Lock()
			machineIDToState[id] = "started"
			bg.stateLock.Unlock()
		}(gm.leasableMachine)
	}

	for {
		if bg.allMachinesStarted(machineIDToState) {
			render()
			return nil
		}

		if bg.isAborted() {
			return ErrAborted
		}

		select {
		case <-wait.C:
			return ErrWaitTimeout
		case err := <-errChan:
			return err
		default:
			time.Sleep(90 * time.Millisecond)
			render()
		}
	}
}

func (bg *blueGreen) changeDetected(a, b map[string]string) bool {
	for key := range a {
		if a[key] != b[key] {
			return true
		}
	}
	return false
}

func (bg *blueGreen) renderMachineHealthchecks(state map[string]*fly.HealthCheckStatus) func() {
	firstRun := true

	previousView := map[string]string{}

	return func() {
		currentView := map[string]string{}
		rows := []string{}
		bg.healthLock.RLock()
		for id, value := range state {
			status := "unchecked"
			if value.Total != 0 {
				status = fmt.Sprintf("%d/%d passing", value.Passing, value.Total)
			}

			currentView[id] = status
			rows = append(rows, fmt.Sprintf("  Machine %s - %s", bg.colorize.Bold(id), bg.colorize.Green(status)))
		}
		bg.healthLock.RUnlock()

		if !firstRun && bg.changeDetected(currentView, previousView) {
			bg.clearLinesAbove(len(rows))
		}

		sort.Strings(rows)

		if bg.changeDetected(currentView, previousView) {
			fmt.Fprintf(bg.io.ErrOut, "%s\n", strings.Join(rows, "\n"))
			previousView = currentView
		}

		firstRun = false
	}
}

func (bg *blueGreen) allMachinesHealthy(stateMap map[string]*fly.HealthCheckStatus) bool {
	passed := 0

	bg.healthLock.RLock()
	for _, v := range stateMap {
		// we initialize all machine ids with an empty struct, so all fields are zero'd on init.
		// without v.hcs.Total != 0, the first call to this function will pass since 0 == 0
		if v.Total == 0 {
			continue
		}

		if v.Passing == v.Total {
			passed += 1
		}
	}
	bg.healthLock.RUnlock()

	return passed == len(stateMap)
}

func (bg *blueGreen) WaitForGreenMachinesToBeHealthy(ctx context.Context) error {
	ctx, span := tracing.GetTracer().Start(ctx, "green_machines_health_wait")
	defer span.End()

	wait := time.NewTicker(bg.timeout)
	machineIDToHealthStatus := map[string]*fly.HealthCheckStatus{}
	errChan := make(chan error)
	render := bg.renderMachineHealthchecks(machineIDToHealthStatus)

	for _, gm := range bg.greenMachines {
		if gm.launchInput.SkipLaunch {
			machineIDToHealthStatus[gm.leasableMachine.FormattedMachineId()] = &fly.HealthCheckStatus{Total: 1, Passing: 1}
			continue
		}

		// in some cases, not all processes have healthchecks setup
		// eg. processes that run background workers, etc.
		// there's no point checking for health, a started state is enough
		if len(gm.leasableMachine.Machine().Checks) == 0 {
			continue
		}

		machineIDToHealthStatus[gm.leasableMachine.FormattedMachineId()] = &fly.HealthCheckStatus{}
	}

	for _, gm := range bg.greenMachines {
		if gm.launchInput.SkipLaunch {
			continue
		}

		// in some cases, not all processes have healthchecks setup
		// eg. processes that run background workers, etc.
		// there's no point checking for health, a started state is enough
		if len(gm.leasableMachine.Machine().Checks) == 0 {
			continue
		}

		go func(m machine.LeasableMachine) {
			waitCtx, cancel := context.WithTimeout(ctx, bg.timeout)
			defer cancel()

			interval, gracePeriod := m.GetMinIntervalAndMinGracePeriod()

			time.Sleep(gracePeriod)

			for {
				updateMachine, err := bg.flaps.Get(waitCtx, m.Machine().ID)

				switch {
				case waitCtx.Err() != nil:
					errChan <- waitCtx.Err()
					return
				case err != nil:
					errChan <- err
					return
				}

				status := updateMachine.TopLevelChecks()
				bg.healthLock.Lock()
				machineIDToHealthStatus[m.FormattedMachineId()] = status
				bg.healthLock.Unlock()

				if (status.Total == status.Passing) && (status.Total != 0) {
					return
				}

				time.Sleep(interval)
			}
		}(gm.leasableMachine)
	}

	for {

		if bg.allMachinesHealthy(machineIDToHealthStatus) {
			render()
			break
		}

		if bg.isAborted() {
			return ErrAborted
		}

		select {
		case err := <-errChan:
			return err
		case <-wait.C:
			return ErrWaitTimeout
		default:
			time.Sleep(90 * time.Millisecond)
			render()
		}
	}

	return nil
}

func (bg *blueGreen) MarkGreenMachinesAsReadyForTraffic(ctx context.Context) error {
	ctx, span := tracing.GetTracer().Start(ctx, "mark_green_machines_for_traffic")
	defer span.End()

	p := pool.New().
		WithErrors().
		WithFirstError().
		WithMaxGoroutines(bg.maxConcurrent)
	for _, gm := range bg.greenMachines.machines() {
		gm := gm
		p.Go(func() error {
			if bg.isAborted() {
				return ErrAborted
			}
			err := bg.flaps.Uncordon(ctx, gm.Machine().ID, "")
			if err != nil {
				return err
			}

			fmt.Fprintf(bg.io.ErrOut, "  Machine %s now ready\n", bg.colorize.Bold(gm.FormattedMachineId()))
			return nil
		})
	}

	return p.Wait()
}

func (bg *blueGreen) CordonBlueMachines(ctx context.Context) error {
	ctx, span := tracing.GetTracer().Start(ctx, "cordon_blue_machines")
	defer span.End()

	p := pool.New().
		WithErrors().
		WithFirstError().
		WithMaxGoroutines(bg.maxConcurrent)
	for _, gm := range bg.blueMachines {
		gm := gm
		p.Go(func() error {
			if bg.isAborted() {
				return ErrAborted
			}
			err := gm.leasableMachine.Cordon(ctx)
			if err != nil {
				// Just let the user know, it's not a critical error
				fmt.Fprintf(bg.io.ErrOut, "  Failed to cordon machine %s: %v\n", bg.colorize.Bold(gm.leasableMachine.FormattedMachineId()), err)
				return nil
			}

			fmt.Fprintf(bg.io.ErrOut, "  Machine %s cordoned\n", bg.colorize.Bold(gm.leasableMachine.FormattedMachineId()))
			return nil
		})
	}
	return p.Wait()
}

func (bg *blueGreen) StopBlueMachines(ctx context.Context) error {
	ctx, span := tracing.GetTracer().Start(ctx, "stop_blue_machines")
	defer span.End()

	p := pool.New().
		WithErrors().
		WithFirstError().
		WithMaxGoroutines(bg.maxConcurrent)
	for _, gm := range bg.blueMachines {
		gm := gm
		p.Go(func() error {
			if bg.isAborted() {
				return ErrAborted
			}
			err := gm.leasableMachine.Stop(ctx, bg.stopSignal)
			if err != nil {
				// Just let the user know, it's not a critical error as we are gonna destroy the
				// machines with force later
				fmt.Fprintf(bg.io.ErrOut, "  Failed to stop machine %s: %v\n", bg.colorize.Bold(gm.leasableMachine.FormattedMachineId()), err)
				return nil
			}
			return nil
		})
	}
	return p.Wait()
}

func (bg *blueGreen) WaitForBlueMachinesToBeStopped(ctx context.Context) error {
	ctx, span := tracing.GetTracer().Start(ctx, "blue_machines_stop_wait")
	defer span.End()

	wait := time.NewTicker(bg.timeout)
	machineIDToState := map[string]string{}
	for _, gm := range bg.blueMachines.machines() {
		machineIDToState[gm.FormattedMachineId()] = gm.Machine().State
	}

	render := bg.renderMachineStates(machineIDToState)
	errChan := make(chan error)

	var done atomic.Uint32
	for _, gm := range bg.blueMachines {
		id := gm.leasableMachine.FormattedMachineId()

		go func(lm machine.LeasableMachine) {
			err := machine.WaitForStartOrStop(ctx, lm.Machine(), "stop", bg.timeout)
			if err != nil {
				errChan <- fmt.Errorf("failed to stop machine %s: %v", lm.FormattedMachineId(), err)
			} else {
				bg.stateLock.Lock()
				machineIDToState[id] = "stopped"
				bg.stateLock.Unlock()
			}
			done.Add(1)
		}(gm.leasableMachine)
	}

	var merr *multierror.Error
	for {
		if done.Load() == uint32(len(bg.blueMachines)) {
			return merr.ErrorOrNil()
		}

		if bg.isAborted() {
			return ErrAborted
		}

		select {
		case <-wait.C:
			return ErrWaitTimeout
		case err := <-errChan:
			// Collect all the errors to report later. Treat them as not fatal as we are gonna
			// destroy the machines later anyway
			merr = multierror.Append(merr, err)
		default:
			time.Sleep(90 * time.Millisecond)
			render()
		}
	}
}

func (bg *blueGreen) DestroyBlueMachines(ctx context.Context) error {
	ctx, span := tracing.GetTracer().Start(ctx, "destroy_blue_machines")
	defer span.End()

	p := pool.New().
		WithErrors().
		WithFirstError().
		WithMaxGoroutines(bg.maxConcurrent)

	var mu sync.Mutex
	for _, gm := range bg.blueMachines {
		gm := gm
		p.Go(func() error {
			if bg.isAborted() {
				return ErrAborted
			}

			err := gm.leasableMachine.Destroy(ctx, true)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				bg.hangingBlueMachines = append(bg.hangingBlueMachines, gm.launchInput.ID)
				return nil
			}

			fmt.Fprintf(bg.io.ErrOut, "  Machine %s destroyed\n", bg.colorize.Bold(gm.leasableMachine.FormattedMachineId()))
			return nil
		})
	}

	if err := p.Wait(); err != nil {
		return err
	}

	return nil
}

func (bg *blueGreen) attachCustomTopLevelChecks() {
	for _, entry := range bg.blueMachines {
		for _, service := range entry.launchInput.Config.Services {
			servicePort := service.InternalPort
			serviceProtocol := service.Protocol

			for _, check := range service.Checks {
				cc := fly.MachineCheck{
					Port:              check.Port,
					Type:              check.Type,
					Interval:          check.Interval,
					Timeout:           check.Timeout,
					GracePeriod:       check.GracePeriod,
					HTTPMethod:        check.HTTPMethod,
					HTTPPath:          check.HTTPPath,
					HTTPProtocol:      check.HTTPProtocol,
					HTTPSkipTLSVerify: check.HTTPSkipTLSVerify,
					HTTPHeaders:       check.HTTPHeaders,
				}

				if cc.Port == nil {
					cc.Port = &servicePort
				}

				if cc.Type == nil {
					cc.Type = &serviceProtocol
				}

				if entry.launchInput.Config.Checks == nil {
					entry.launchInput.Config.Checks = make(map[string]fly.MachineCheck)
				}
				entry.launchInput.Config.Checks[fmt.Sprintf("bg_deployments_%s", *check.Type)] = cc
			}
		}
	}
}

func (bg *blueGreen) Deploy(ctx context.Context) error {
	ctx, span := tracing.GetTracer().Start(ctx, "bluegreen")
	defer span.End()

	defer bg.ctrlcHook.Done()

	if bg.isAborted() {
		return ErrAborted
	}

	canPerform, err := bg.apiClient.CanPerformBluegreenDeployment(ctx, bg.appConfig.AppName)
	if err != nil {
		tracing.RecordError(span, err, "failed to validate deployment")
		return err
	}

	span.SetAttributes(attribute.Bool("can_perform", canPerform))

	if !canPerform {
		tracing.RecordError(span, ErrOrgLimit, "failed to deploy, orglimit")
		return ErrOrgLimit
	}

	fmt.Fprintf(bg.io.ErrOut, "\nVerifying if app can be safely deployed \n")

	err = bg.DetectMultipleImageVersions(ctx)
	if err != nil {
		tracing.RecordError(span, ErrMultipleImageVersions, "failed to deploy, multiple_versions")
		return err
	}

	bg.attachCustomTopLevelChecks()

	totalChecks := 0
	for _, entry := range bg.blueMachines {
		if len(entry.launchInput.Config.Checks) == 0 {
			fmt.Fprintf(bg.io.ErrOut, "\n[WARN] Machine %s doesn't have healthchecks setup. We won't check its health.", entry.leasableMachine.FormattedMachineId())
			continue
		}

		totalChecks++
	}

	if totalChecks == 0 && len(bg.blueMachines) != 0 {
		fmt.Fprintf(bg.io.ErrOut, "\n\nYou need to define at least 1 check in order to use blue-green deployments. Refer to https://fly.io/docs/reference/configuration/#services-tcp_checks\n")
		return ErrValidationError
	}

	fmt.Fprintf(bg.io.ErrOut, "\nCreating green machines\n")
	if err := bg.CreateGreenMachines(ctx); err != nil {
		return errors.Join(err, ErrCreateGreenMachine)
	}

	if bg.isAborted() {
		return ErrAborted
	}

	// because computers are too fast and everyone deserves a break sometimes
	time.Sleep(300 * time.Millisecond)

	fmt.Fprintf(bg.io.ErrOut, "\nWaiting for all green machines to start\n")
	if err := bg.WaitForGreenMachinesToBeStarted(ctx); err != nil {
		tracing.RecordError(span, err, "failed to wait for start")
		return errors.Join(err, ErrWaitForStartedState)
	}

	if bg.isAborted() {
		return ErrAborted
	}

	fmt.Fprintf(bg.io.ErrOut, "\nWaiting for all green machines to be healthy\n")
	if err := bg.WaitForGreenMachinesToBeHealthy(ctx); err != nil {
		tracing.RecordError(span, err, "failed to wait for health")
		return errors.Join(err, ErrWaitForHealthy)
	}

	if bg.isAborted() {
		return ErrAborted
	}

	fmt.Fprintf(bg.io.ErrOut, "\nMarking green machines as ready\n")
	if err := bg.MarkGreenMachinesAsReadyForTraffic(ctx); err != nil {
		tracing.RecordError(span, err, "failed to mark as ready for traffic")
		return errors.Join(err, ErrMarkReadyForTraffic)
	}

	// after this point, a rollback should never delete green machines.
	bg.rollbackLog.canDeleteGreenMachines = false

	if bg.isAborted() {
		return ErrAborted
	}

	fmt.Fprintf(bg.io.ErrOut, "\nCheckpointing deployment, this may take a few seconds...\n")
	if err := bg.TagBlueMachinesAsSafeForDeletion(ctx); err != nil {
		tracing.RecordError(span, err, "failed to mark as ready for traffic")
		return errors.Join(err, ErrTagForDeletion)
	}

	if bg.isAborted() {
		return ErrAborted
	}

	// Wait a bit to let fly-proxy see the new machines
	fmt.Fprintf(bg.io.ErrOut, "\nWaiting before cordoning all blue machines\n")
	if bg.sleepAbortable(bg.waitBeforeCordon) {
		return ErrAborted
	}

	// Stop fly-proxy from sending new traffic to the old machines
	if err := bg.CordonBlueMachines(ctx); err != nil {
		tracing.RecordError(span, err, "failed to cordon blue machines")
		return errors.Join(err, ErrCordonBlueMachines)
	}

	if bg.isAborted() {
		return ErrAborted
	}

	// Wait a bit to let fly-proxy forget about the old machines
	fmt.Fprintf(bg.io.ErrOut, "\nWaiting before stopping all blue machines\n")
	if bg.sleepAbortable(bg.waitBeforeStop) {
		return ErrAborted
	}

	// Stop blue machine first to let the app react to SIGTERM and gracefully
	// terminate existing connections
	fmt.Fprintf(bg.io.ErrOut, "\nStopping all blue machines\n")
	if err := bg.StopBlueMachines(ctx); err != nil {
		tracing.RecordError(span, err, "failed to stop blue machines")
		return errors.Join(err, ErrStopBlueMachines)
	}

	fmt.Fprintf(bg.io.ErrOut, "\nWaiting for all blue machines to stop\n")
	if err := bg.WaitForBlueMachinesToBeStopped(ctx); err != nil {
		tracing.RecordError(span, err, "failed to wait for stop")
		var merr *multierror.Error
		if errors.As(err, &merr) {
			fmt.Fprintf(bg.io.ErrOut, "\nFailed to stop some machines:\n")
			for err := range merr.Errors {
				fmt.Fprintf(bg.io.ErrOut, "  %v\n", err)
			}
		} else {
			return errors.Join(err, ErrWaitForStoppedState)
		}
	}

	fmt.Fprintf(bg.io.ErrOut, "\nDestroying all blue machines\n")
	if err := bg.DestroyBlueMachines(ctx); err != nil {
		tracing.RecordError(span, err, "failed to destroy blue machines")
		return errors.Join(err, ErrDestroyBlueMachines)
	}

	fmt.Fprintf(bg.io.ErrOut, "\nDeployment Complete\n")
	return nil
}

func getZombies(ids map[string]bool) (map[string]bool, error) {
	numbers := []int{}
	for str := range ids {
		num, err := strconv.Atoi(str)
		if err != nil {
			return ids, err
		}
		numbers = append(numbers, num)
	}

	sort.Ints(numbers)

	delete(ids, fmt.Sprint(numbers[len(numbers)-1]))
	return ids, nil
}

// detects zombie machines, deletes them, and update the list of machines to be updated
func (bg *blueGreen) DeleteZombiesFromPreviousDeployment(ctx context.Context) error {
	tags := map[string]bool{}

	for _, mach := range bg.blueMachines {
		if mach.launchInput.Config.Metadata[fly.MachineConfigMetadataKeyFlyctlBGTag] == "" {
			mach.launchInput.Config.Metadata[fly.MachineConfigMetadataKeyFlyctlBGTag] = "-1"
		}
		tags[mach.launchInput.Config.Metadata[fly.MachineConfigMetadataKeyFlyctlBGTag]] = true
	}

	if len(tags) == 1 {
		fmt.Fprintf(bg.io.ErrOut, "  No hanging machines from a failed previous deployment\n")
		return nil
	}

	zombies, err := getZombies(tags)
	if err != nil {
		return err
	}

	for _, mach := range bg.blueMachines {
		if bg.isAborted() {
			return ErrAborted
		}

		tag := mach.launchInput.Config.Metadata[fly.MachineConfigMetadataKeyFlyctlBGTag]
		if ok := zombies[tag]; !ok {
			continue
		}

		deleteFunc := func() error {
			return mach.leasableMachine.Destroy(ctx, true)
		}

		err := retry.Do(deleteFunc,
			retry.Context(ctx),
			retry.Attempts(3),
			retry.Delay(2*time.Second),
			retry.DelayType(retry.FixedDelay),
		)
		if err != nil {
			return err
		}

		fmt.Fprintf(bg.io.ErrOut, "  Zombie Machine %s destroyed [%s]\n", bg.colorize.Bold(mach.leasableMachine.FormattedMachineId()), mach.launchInput.Config.Metadata[fly.MachineConfigMetadataKeyFlyctlBGTag])
	}

	nonZombies := []*machineUpdateEntry{}
	for _, mach := range bg.blueMachines {
		tag := mach.launchInput.Config.Metadata[fly.MachineConfigMetadataKeyFlyctlBGTag]
		if zombies[tag] {
			continue
		}
		nonZombies = append(nonZombies, mach)
	}

	bg.blueMachines = nonZombies

	return nil
}

func (bg *blueGreen) CanDestroyGreenMachines(err error) bool {
	validErrors := []error{
		ErrCreateGreenMachine,
		ErrWaitForStartedState,
		ErrWaitForHealthy,
		ErrMarkReadyForTraffic,
	}

	for _, validError := range validErrors {
		if errors.Is(err, validError) {
			return true
		}
	}

	// this ensures aborts after green machines are healthy don't delete green machines
	if errors.Is(err, ErrAborted) && bg.rollbackLog.canDeleteGreenMachines {
		return true
	}

	return false
}

func (bg *blueGreen) Rollback(ctx context.Context, err error) error {
	ctx, span := tracing.GetTracer().Start(ctx, "rollback", trace.WithAttributes(
		attribute.Bool("rollback_disabled", bg.rollbackLog.disableRollback),
		attribute.Bool("can_delete_green_machines", bg.rollbackLog.canDeleteGreenMachines),
		attribute.String("deployment_error", err.Error()),
	))
	defer span.End()

	if bg.rollbackLog.disableRollback {
		return nil
	}

	if errors.Is(err, ErrDestroyBlueMachines) {
		fmt.Fprintf(bg.io.ErrOut, "\nFailed to destroy blue machines (%s)\n", strings.Join(bg.hangingBlueMachines, ","))
		fmt.Fprintf(bg.io.ErrOut, "\nYou can destroy them using `fly machines destroy --force <id>`")
		return nil
	}

	if bg.CanDestroyGreenMachines(err) {
		fmt.Fprintf(bg.io.ErrOut, "\nRolling back failed deployment\n")
		for _, mach := range bg.greenMachines.machines() {
			err := mach.Destroy(ctx, true)
			if err != nil {
				tracing.RecordError(span, err, "failed to destroy green machine")
				return err
			}
			fmt.Fprintf(bg.io.ErrOut, "  Deleted machine %s\n", bg.colorize.Bold(mach.FormattedMachineId()))
		}
	}

	return nil
}

// This method aggregates images for machines in an app
// If they are greater than 1, it suggest how to remove them and unblock the app
// It also uses the bg_deployment_tag to suggest blue machines that can be safely deleted.
func (bg *blueGreen) DetectMultipleImageVersions(ctx context.Context) error {
	imageToMachineIDs := map[string][]string{}
	safeToDelete := map[string]int{}

	for _, mach := range bg.blueMachines {
		image := mach.leasableMachine.Machine().ImageRefWithVersion()
		imageToMachineIDs[image] = append(imageToMachineIDs[image], mach.leasableMachine.Machine().ID)
		if mach.launchInput.Config.Metadata[fly.MachineConfigMetadataKeyFlyctlBGTag] == safeToDestroyValue {
			safeToDelete[image] = 1
		}
	}

	if len(imageToMachineIDs) == 1 {
		return nil
	}

	fmt.Fprintf(bg.io.ErrOut, "\n  Found %d different images in your app (for bluegreen to work, all machines need to run a single image)\n", len(imageToMachineIDs))
	for image, ids := range imageToMachineIDs {
		fmt.Fprintf(bg.io.ErrOut, "    [x] %s - %v machine(s) (%s)\n", image, len(ids), strings.Join(imageToMachineIDs[image], ","))
	}

	if len(safeToDelete) > 0 {
		fmt.Fprintf(bg.io.ErrOut, "\n  These image(s) can be safely destroyed:\n")
		for image := range safeToDelete {
			fmt.Fprintf(bg.io.ErrOut, "    [x] %s - %v machine(s) ('fly machines destroy --force --image=%s')\n", image, len(imageToMachineIDs[image]), image)
		}
	}

	fmt.Fprintf(bg.io.ErrOut, "\n  Here's how to fix your app so deployments can go through:\n")
	fmt.Fprintf(bg.io.ErrOut, "    1. Find all the unwanted image versions from the list above.\n")
	fmt.Fprintf(bg.io.ErrOut, "       Use 'fly machines list' and 'fly releases --image' to help determine unwanted images.\n")
	fmt.Fprintf(bg.io.ErrOut, "    2. For each unwanted image version, run 'fly machines destroy --force --image=<insert-image-version>'\n")
	fmt.Fprintf(bg.io.ErrOut, "    3. Retry the deployment with 'fly deploy'\n")
	fmt.Fprintf(bg.io.ErrOut, "\n")

	return ErrMultipleImageVersions
}

// This method tags blue-machines with a safe to destroy value.
// This way, a user can easily remove blue machines that are hanging around from deployment.
func (bg *blueGreen) TagBlueMachinesAsSafeForDeletion(ctx context.Context) error {
	ctx, span := tracing.GetTracer().Start(ctx, "tag_blue_machines")
	defer span.End()

	p := pool.New().WithErrors().WithFirstError().WithMaxGoroutines(bg.maxConcurrent)
	for _, mach := range bg.blueMachines {
		mach := mach
		p.Go(func() error {
			return mach.leasableMachine.SetMetadata(ctx, fly.MachineConfigMetadataKeyFlyctlBGTag, "safe_to_destroy")
		})
	}

	return p.Wait()
}
