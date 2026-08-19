package deploy

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"errors"

	"github.com/avast/retry-go/v4"
	"github.com/hashicorp/go-multierror"
	"github.com/oklog/ulid/v2"
	"github.com/sourcegraph/conc/pool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
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

	// flyctlBGLaunchIDMetadataKey is a per-launch idempotency tag that flyctl
	// writes into the machine config's metadata before calling flaps.Launch.
	// Each intended green machine gets a unique value. When Launch fails with
	// an ambiguous transient error (like a 408 propagated from flyd via flaps),
	// the retry loop looks the machine up by this key to detect a silent
	// success and avoid creating a duplicate machine. The key is intentionally
	// namespaced under "fly_flyctl_" so it can coexist with any user metadata.
	flyctlBGLaunchIDMetadataKey = "fly_flyctl_bluegreen_launch_id"
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
	app                 *flaps.App

	rollbackLog RollbackLog

	waitBeforeStop   time.Duration
	waitBeforeCordon time.Duration

	uncordonRetryAttempts uint
	uncordonRetryDelay    time.Duration

	// tagRetryAttempts / tagRetryDelay control the back-off used when
	// TagBlueMachinesAsSafeForDeletion writes the "safe_to_destroy" metadata
	// tag on blue machines. Flaps' metadata endpoint can return transient 408
	// (context.DeadlineExceeded talking to flyd) and we don't want a single
	// transient failure to strand blue and green versions serving traffic
	// side-by-side (see: the checkpointing step in Deploy).
	tagRetryAttempts uint
	tagRetryDelay    time.Duration

	// launchRetryAttempts / launchRetryDelay control the back-off used when
	// CreateGreenMachines retries a Launch that failed with a transient error.
	// Retries are safe because each launch input carries a unique per-machine
	// idempotency tag (flyctlBGLaunchIDMetadataKey) that lets the retry loop
	// detect a "silent success" (a Launch that hit an ambiguous 408/5xx but
	// still committed the machine on the flyd side) by listing machines with
	// that tag before creating a new one.
	launchRetryAttempts uint
	launchRetryDelay    time.Duration

	// launchLookupDelay is the pause between a failed Launch attempt and the
	// idempotency lookup that follows it. It gives flaps' backing store a
	// moment to reflect a machine that a silent-success Launch just committed,
	// which reduces (but does not eliminate) the race where a retry could
	// otherwise create a duplicate.
	launchLookupDelay time.Duration

	// imageRefRetryAttempts / imageRefRetryDelay control the back-off used when
	// DetectMultipleImageVersions re-fetches a machine whose ImageRef came back
	// empty from the list API (e.g. due to a transient API error).
	imageRefRetryAttempts uint
	imageRefRetryDelay    time.Duration
}

// hostIsOk reports whether the machine's host is confirmed healthy. Anything
// else — "unreachable", "unknown", or unset — means the host cannot be
// trusted to respond: such machines are excluded from image verification and
// from the cordon/stop stages, and are force-destroyed instead.
func hostIsOk(m *fly.Machine) bool {
	return m.HostStatus == fly.HostStatusOk
}

// machineHasConfiguredChecks returns true if the machine config has any health
// checks defined — either at the top-level or inside a service. This is
// intentionally based on the *configuration*, not on the runtime Machine.Checks
// status field, which is empty for freshly-launched machines.
func machineHasConfiguredChecks(cfg *fly.MachineConfig) bool {
	if len(cfg.Checks) > 0 {
		return true
	}
	for _, svc := range cfg.Services {
		if len(svc.Checks) > 0 {
			return true
		}
	}

	return false
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
		app:                 md.app,
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

	bg.uncordonRetryAttempts = 5
	bg.uncordonRetryDelay = 500 * time.Millisecond

	bg.tagRetryAttempts = 5
	bg.tagRetryDelay = 500 * time.Millisecond

	bg.launchRetryAttempts = 3
	bg.launchRetryDelay = 500 * time.Millisecond
	bg.launchLookupDelay = 500 * time.Millisecond

	bg.imageRefRetryAttempts = 3
	bg.imageRefRetryDelay = 1 * time.Second
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

// forceStartRepresentatives returns the set of blue-machine indices whose
// green replacements must be launched with SkipLaunch=false so that each
// process group with configured health checks has at least one machine that
// starts and can be health-verified.
//
// Rule: for each process group with any configured health check, ensure a
// representative will start. Machines whose blue counterpart is already going
// to start naturally (SkipLaunch=false — e.g. it was running when the deploy
// began) satisfy the invariant for free. If no machine in the group would
// naturally start (e.g. auto_stop_machines turned them all off), promote the
// first machine in the group.
//
// Machines outside the returned set inherit their blue's SkipLaunch value,
// so a stopped blue produces a stopped green — mirroring the app's
// pre-deploy state rather than unnecessarily waking up idle workers.
func (bg *blueGreen) forceStartRepresentatives() map[int]bool {
	groupHasChecks := map[string]bool{}
	groupWillStart := map[string]bool{}
	groupFirstStopped := map[string]int{} // process group -> lowest index of a machine that would stay stopped

	for i, mach := range bg.blueMachines {
		cfg := mach.launchInput.Config
		pg := cfg.ProcessGroup()

		if machineHasConfiguredChecks(cfg) {
			groupHasChecks[pg] = true
		}
		if !mach.launchInput.SkipLaunch {
			groupWillStart[pg] = true
		} else if _, seen := groupFirstStopped[pg]; !seen {
			groupFirstStopped[pg] = i
		}
	}

	forceStart := map[int]bool{}
	for pg := range groupHasChecks {
		if groupWillStart[pg] {
			continue
		}
		if idx, ok := groupFirstStopped[pg]; ok {
			forceStart[idx] = true
		}
	}

	return forceStart
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

	// Decide upfront which green machines must be force-started so each
	// process group with health checks has at least one machine to poll.
	// Everything else inherits SkipLaunch from its blue counterpart.
	forceStart := bg.forceStartRepresentatives()

	var lock sync.Mutex
	p := pool.New().
		WithErrors().
		WithFirstError().
		WithMaxGoroutines(createConcurrency)
	for i, mach := range bg.blueMachines {
		p.Go(func() error {
			if bg.isAborted() {
				return ErrAborted
			}

			launchInput := mach.launchInput
			launchInput.SkipServiceRegistration = true
			if forceStart[i] {
				launchInput.SkipLaunch = false
			}
			launchInput.Config.Metadata[fly.MachineConfigMetadataKeyFlyctlBGTag] = bg.timestamp
			// A per-machine ULID lets us safely retry Launch on ambiguous
			// transient errors (e.g. 408 from flaps): the retry loop first
			// looks up any machine flaps has already committed with this key
			// before attempting another Launch, so a "silent success" doesn't
			// turn into a duplicate green machine.
			launchID := ulid.Make().String()
			launchInput.Config.Metadata[flyctlBGLaunchIDMetadataKey] = launchID

			newMachineRaw, err := bg.launchGreenMachineWithRetry(ctx, launchInput, launchID)
			if err != nil {
				tracing.RecordError(span, err, "failed to launch machine")

				return err
			}

			greenMachine := machine.NewLeasableMachine(bg.flaps, bg.io, bg.app.Name, newMachineRaw, true)
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
			err := machine.WaitForStartOrStop(ctx, bg.app.Name, lm.Machine(), "start", bg.timeout)
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
		if !machineHasConfiguredChecks(gm.launchInput.Config) {
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
		if !machineHasConfiguredChecks(gm.launchInput.Config) {
			continue
		}

		go func(m machine.LeasableMachine) {
			waitCtx, cancel := context.WithTimeout(ctx, bg.timeout)
			defer cancel()

			interval, gracePeriod := m.GetMinIntervalAndMinGracePeriod()

			time.Sleep(gracePeriod)

			for {
				updateMachine, err := bg.flaps.Get(waitCtx, bg.app.Name, m.Machine().ID)

				switch {
				case waitCtx.Err() != nil:
					errChan <- waitCtx.Err()

					return
				case err != nil:
					errChan <- err

					return
				}

				status := updateMachine.AllHealthChecks()
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
		p.Go(func() error {
			if bg.isAborted() {
				return ErrAborted
			}
			err := retry.Do(
				func() error {
					return bg.flaps.Uncordon(ctx, bg.app.Name, gm.Machine().ID, "")
				},
				retry.Context(ctx),
				retry.Attempts(bg.uncordonRetryAttempts),
				retry.Delay(bg.uncordonRetryDelay),
				retry.MaxDelay(30*time.Second),
				retry.DelayType(retry.BackOffDelay),
				retry.OnRetry(func(n uint, err error) {
					fmt.Fprintf(bg.io.ErrOut, "  Retrying uncordon for machine %s (attempt %d/%d): %v\n",
						bg.colorize.Bold(gm.FormattedMachineId()), n+2, bg.uncordonRetryAttempts, err)
				}),
			)
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
		// A machine on a non-ok host can't respond to a cordon; it gets
		// force-destroyed at the end of the deployment instead.
		if !hostIsOk(gm.leasableMachine.Machine()) {
			continue
		}
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
		// A machine on a non-ok host can't react to a stop signal; it gets
		// force-destroyed at the end of the deployment instead.
		if !hostIsOk(gm.leasableMachine.Machine()) {
			continue
		}
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

	// Machines on non-ok hosts were never stopped — waiting on them would only
	// burn the timeout; they get force-destroyed instead.
	waitable := machineUpdateEntries{}
	for _, gm := range bg.blueMachines {
		if hostIsOk(gm.leasableMachine.Machine()) {
			waitable = append(waitable, gm)
		}
	}

	wait := time.NewTicker(bg.timeout)
	machineIDToState := map[string]string{}
	for _, gm := range waitable.machines() {
		machineIDToState[gm.FormattedMachineId()] = gm.Machine().State
	}

	render := bg.renderMachineStates(machineIDToState)
	errChan := make(chan error)

	var done atomic.Uint32
	for _, gm := range waitable {
		id := gm.leasableMachine.FormattedMachineId()

		go func(lm machine.LeasableMachine) {
			err := machine.WaitForStartOrStop(ctx, bg.app.Name, lm.Machine(), "stop", bg.timeout)
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
		if done.Load() == uint32(len(waitable)) {
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
		p.Go(func() error {
			if bg.isAborted() {
				return ErrAborted
			}

			var err error
			if hostIsOk(gm.leasableMachine.Machine()) {
				err = gm.leasableMachine.Destroy(ctx, true)
			} else {
				// No lease exists for a machine on a non-ok host (lease
				// acquisition skips them), so bypass the leasable wrapper and
				// issue the destroy straight through the flaps API with
				// kill=true and no nonce — the exact call behind
				// `fly machine destroy --force`. A stale nonce from the list
				// response could otherwise make flaps reject the destroy.
				err = bg.flaps.Destroy(ctx, bg.app.Name, fly.RemoveMachineInput{
					ID:   gm.leasableMachine.Machine().ID,
					Kill: true,
				}, "")
			}

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

	// Machines that could not be destroyed (typically because their host is
	// down) would otherwise linger silently. Tell the user how to finish the
	// cleanup by hand.
	if len(bg.hangingBlueMachines) > 0 {
		fmt.Fprintf(bg.io.ErrOut, "\n  Failed to destroy %d machine(s). Remove them manually with:\n\n    %s\n\n",
			len(bg.hangingBlueMachines), formatDestroyCommand(bg.appConfig.AppName, bg.hangingBlueMachines))
	}

	return nil
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

	totalMachinesWithChecks := 0
	for _, entry := range bg.blueMachines {
		machineChecks := len(entry.launchInput.Config.Checks)

		// Also count service-level checks
		for _, service := range entry.launchInput.Config.Services {
			machineChecks += len(service.Checks)
		}

		if machineChecks == 0 {
			fmt.Fprintf(bg.io.ErrOut, "\n[WARN] Machine %s doesn't have healthchecks setup. We won't check its health.", entry.leasableMachine.FormattedMachineId())

			continue
		}

		totalMachinesWithChecks++
	}

	if totalMachinesWithChecks == 0 && len(bg.blueMachines) != 0 {
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

// imageRefIsEmpty reports whether a machine's ImageRef fields are both empty,
// which is how the API signals that full machine data is unavailable (e.g. the
// host is unreachable). ImageRefWithVersion() would return ":" in this case,
// which is not a real image identifier.
func imageRefIsEmpty(m *fly.Machine) bool {
	return m.ImageRef.Repository == "" && m.ImageRef.Tag == ""
}

// refreshMachineImageRef fetches fresh data for a single machine and retries
// on transient API errors using exponential backoff (circuit-break after
// imageRefRetryAttempts). A successful response that still carries an empty
// ImageRef is a stable platform signal (the host is unreachable) and is
// returned to the caller as-is — retrying won't change that outcome.
func (bg *blueGreen) refreshMachineImageRef(ctx context.Context, machineID string) (*fly.Machine, error) {
	var fresh *fly.Machine

	err := retry.Do(
		func() error {
			var apiErr error
			fresh, apiErr = bg.flaps.Get(ctx, bg.app.Name, machineID)

			return apiErr // only retry on hard API errors, not on empty-ImageRef responses
		},
		retry.Context(ctx),
		retry.Attempts(bg.imageRefRetryAttempts),
		retry.Delay(bg.imageRefRetryDelay),
		retry.MaxDelay(5*time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.OnRetry(func(n uint, err error) {
			fmt.Fprintf(bg.io.ErrOut, "  Retrying image lookup for machine %s (attempt %d/%d): %v\n",
				machineID, n+1, bg.imageRefRetryAttempts, err)
		}),
	)

	return fresh, err
}

// formatDestroyCommand returns a ready-to-run destroy command for one or more
// unreachable machines. When there are multiple IDs the command uses backslash
// continuation so users can copy-paste each ID individually or the whole block.
func formatDestroyCommand(appName string, machineIDs []string) string {
	base := fmt.Sprintf("fly machine destroy --force -a %s", appName)
	if len(machineIDs) == 1 {
		return base + " " + machineIDs[0]
	}
	lines := make([]string, len(machineIDs))
	for i, id := range machineIDs {
		lines[i] = "  " + id
	}

	return base + " \\\n" + strings.Join(lines, " \\\n")
}

func (bg *blueGreen) DetectMultipleImageVersions(ctx context.Context) error {
	imageToMachineIDs := map[string][]string{}
	safeToDelete := map[string]int{}
	var unreachableIDs []string // machines whose image could not be determined

	for _, mach := range bg.blueMachines {
		m := mach.leasableMachine.Machine()

		// The platform doesn't report this machine's host as ok: its image
		// cannot be verified and its config is incomplete. Leave it out of the
		// image tally — its green replacement runs the new image on a healthy
		// host regardless of what this machine was running.
		if !hostIsOk(m) {
			unreachableIDs = append(unreachableIDs, m.ID)

			continue
		}

		// If the list API returned incomplete data for this machine (empty ImageRef),
		// attempt a targeted re-fetch with exponential backoff before drawing any
		// conclusions. This recovers transient errors and avoids misidentifying a
		// lookup failure as an image-version conflict.
		if imageRefIsEmpty(m) {
			fmt.Fprintf(bg.io.ErrOut, "  Machine %s has no image data — retrying lookup...\n", m.ID)
			freshMachine, err := bg.refreshMachineImageRef(ctx, m.ID)

			if err != nil || !hostIsOk(freshMachine) || imageRefIsEmpty(freshMachine) {
				// Still no image data after retries: treat it like an
				// unreachable host and replace the machine.
				unreachableIDs = append(unreachableIDs, m.ID)

				continue
			}
			m = freshMachine
		}

		image := m.ImageRefWithVersion()
		imageToMachineIDs[image] = append(imageToMachineIDs[image], m.ID)
		if mach.launchInput.Config.Metadata[fly.MachineConfigMetadataKeyFlyctlBGTag] == safeToDestroyValue {
			safeToDelete[image] = 1
		}
	}

	// Unreachable machines never block the deploy: warn and proceed. This is
	// the "080d92df225538 returned ':' " production scenario — pre-existing
	// behavior misreported it as an image-version conflict.
	if len(unreachableIDs) > 0 {
		bg.warnUnreachableMachines(unreachableIDs)
	}

	// Clean state: all reachable machines agree on a single image, or every
	// machine sits on an unreachable host (all get replaced). An app with no
	// machines at all still falls through to the error below, preserving
	// long-standing behavior.
	if len(imageToMachineIDs) == 1 || (len(imageToMachineIDs) == 0 && len(unreachableIDs) > 0) {
		return nil
	}

	// Genuine image-version conflict across reachable machines.
	fmt.Fprintf(bg.io.ErrOut, "\n  Found %d different images in your app (for bluegreen to work, all machines need to run a single image)\n", len(imageToMachineIDs))
	for image, ids := range imageToMachineIDs {
		fmt.Fprintf(bg.io.ErrOut, "    [x] %s - %v machine(s) (%s)\n", image, len(ids), strings.Join(ids, ","))
	}

	if len(safeToDelete) > 0 {
		fmt.Fprintf(bg.io.ErrOut, "\n  These image(s) are from a previous failed deployment and can be safely destroyed:\n")
		for image := range safeToDelete {
			fmt.Fprintf(bg.io.ErrOut, "    [x] %s - %v machine(s) ('fly machines destroy --force --image=%s --app=%s')\n", image, len(imageToMachineIDs[image]), image, bg.appConfig.AppName)
		}
	}

	fmt.Fprintf(bg.io.ErrOut, "\n  Here's how to fix your app so deployments can go through:\n")
	fmt.Fprintf(bg.io.ErrOut, "    1. Find all the unwanted image versions from the list above.\n")
	fmt.Fprintf(bg.io.ErrOut, "       Use 'fly machines list' and 'fly releases --image' to help determine unwanted images.\n")
	fmt.Fprintf(bg.io.ErrOut, "    2. For each unwanted image version, run 'fly machines destroy --force --image=<image-version> --app <app-name>'\n")
	fmt.Fprintf(bg.io.ErrOut, "    3. Retry the deployment with 'fly deploy'\n")
	fmt.Fprintf(bg.io.ErrOut, "\n")

	return ErrMultipleImageVersions
}

// warnUnreachableMachines prints a standout warning when some machines could
// not be reached for image verification. The deployment proceeds: green
// machines are created on healthy hosts and the unreachable blues are
// destroyed, or reported as hanging when the platform cannot destroy them.
func (bg *blueGreen) warnUnreachableMachines(unreachableIDs []string) {
	sep := bg.colorize.Yellow(strings.Repeat("!", 70))
	fmt.Fprintf(bg.io.ErrOut, "\n%s\n", sep)
	fmt.Fprint(bg.io.ErrOut, bg.colorize.Yellow("  WARNING: some machines are on hosts that are not ok — skipping image check for them\n"))
	fmt.Fprintf(bg.io.ErrOut, "\n  %d machine(s) could not be reached to verify their running image:\n", len(unreachableIDs))
	for _, id := range unreachableIDs {
		fmt.Fprintf(bg.io.ErrOut, "    · %s\n", id)
	}
	fmt.Fprintf(bg.io.ErrOut, "\n  Deployment proceeding. These machines will be replaced on healthy hosts\n"+
		"  and force-destroyed at the end of the deployment.\n")
	fmt.Fprintf(bg.io.ErrOut, "%s\n\n", sep)
}

// This method tags blue-machines with a safe to destroy value.
// This way, a user can easily remove blue machines that are hanging around from deployment.
//
// The tag is purely informational: it lets operators identify hanging blue
// machines from a failed previous deployment. The subsequent cordon/stop/
// destroy stages of the deployment do NOT depend on the tag being set, so a
// failure here must not abort the deployment. If we returned an error we'd
// leave the green machines already accepting traffic while the blue machines
// remain live too — two versions serving traffic side-by-side, exactly what
// blue-green deploys are supposed to prevent.
//
// Every SetMetadata call is retried with exponential back-off on transient
// errors (408/timeouts from flyd via flaps, 5xx, common network hiccups).
// SetMetadata is idempotent — writing the same key/value twice is a no-op —
// so retrying is always safe. Any machine that still can't be tagged after
// all retries is reported to the user via a warning; the deploy carries on.
func (bg *blueGreen) TagBlueMachinesAsSafeForDeletion(ctx context.Context) error {
	ctx, span := tracing.GetTracer().Start(ctx, "tag_blue_machines")
	defer span.End()

	var (
		untaggedMu sync.Mutex
		untagged   []string
	)

	p := pool.New().WithMaxGoroutines(bg.maxConcurrent)
	for _, mach := range bg.blueMachines {
		p.Go(func() {
			err := retry.Do(
				func() error {
					return mach.leasableMachine.SetMetadata(ctx, fly.MachineConfigMetadataKeyFlyctlBGTag, safeToDestroyValue)
				},
				retry.Context(ctx),
				retry.Attempts(bg.tagRetryAttempts),
				retry.Delay(bg.tagRetryDelay),
				retry.MaxDelay(10*time.Second),
				retry.DelayType(retry.BackOffDelay),
				retry.RetryIf(isTransientFlapsError),
				retry.LastErrorOnly(true),
				retry.OnRetry(func(n uint, err error) {
					fmt.Fprintf(bg.io.ErrOut, "  Retrying safe-for-deletion tag for machine %s (attempt %d/%d): %v\n",
						bg.colorize.Bold(mach.leasableMachine.FormattedMachineId()), n+2, bg.tagRetryAttempts, err)
				}),
			)
			if err == nil {
				return
			}

			// Failing to tag a machine is non-fatal — the deployment must proceed
			// so that green machines are the only ones serving traffic. We just
			// let the user know so they can manually clean up if a later step
			// also fails.
			tracing.RecordError(span, err, "failed to tag blue machine as safe for deletion")
			fmt.Fprintf(bg.io.ErrOut,
				"  [warn] Could not tag machine %s as safe-for-deletion after %d attempts: %v\n",
				bg.colorize.Bold(mach.leasableMachine.FormattedMachineId()), bg.tagRetryAttempts, err)

			untaggedMu.Lock()
			untagged = append(untagged, mach.leasableMachine.Machine().ID)
			untaggedMu.Unlock()
		})
	}

	p.Wait()

	if len(untagged) > 0 {
		span.SetAttributes(attribute.Int("untagged_blue_machines", len(untagged)))
		span.SetAttributes(attribute.StringSlice("untagged_blue_machine_ids", untagged))
		fmt.Fprintf(bg.io.ErrOut,
			"  [warn] %d blue machine(s) could not be tagged safe-for-deletion. "+
				"The deployment will still proceed; if a later step fails you may need to remove them manually with:\n    %s\n",
			len(untagged), formatDestroyCommand(bg.app.Name, untagged))
	}

	return nil
}

// isTransientFlapsError reports whether an error returned by flaps is worth
// another attempt. It's intentionally strict about "pre-request" transports
// (connection reset/refused, EOF, etc.) but also includes HTTP status codes
// that flaps uses for transient upstream conditions:
//   - 408 Request Timeout: flaps hit a context.DeadlineExceeded talking to flyd
//   - 429 Too Many Requests: rate-limited
//   - 5xx Server Error: transient server-side failure
//
// This classifier must NOT be used with non-idempotent endpoints (like Launch)
// unless the caller only relies on the pre-request substrings, since a 408
// from flaps doesn't tell us whether the upstream side-effect happened.
func isTransientFlapsError(err error) bool {
	if err == nil {
		return false
	}

	// User interrupted / deadline elapsed for the whole operation: don't retry.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var flapsErr *flaps.FlapsError
	if errors.As(err, &flapsErr) {
		switch {
		case flapsErr.ResponseStatusCode == http.StatusRequestTimeout,
			flapsErr.ResponseStatusCode == http.StatusTooManyRequests,
			flapsErr.ResponseStatusCode >= 500 && flapsErr.ResponseStatusCode < 600:
			return true
		}
	}

	return hasTransientNetworkSubstring(err)
}

// launchGreenMachineWithRetry launches a single green machine with client-side
// pseudo-idempotency. flaps' create-machine endpoint doesn't accept an
// Idempotency-Key header, so we simulate one by writing a unique ULID into
// the machine's metadata under flyctlBGLaunchIDMetadataKey before every
// attempt. If a Launch call fails with a transient error we don't know
// whether the machine was actually committed on the flyd side, so before
// retrying we list machines and look for one carrying our launch ID. If we
// find one, we treat the failed Launch as a silent success and return that
// machine — no duplicate is created.
//
// The race we can't fully close is when flaps commits the machine but the
// list-lookup runs before that write propagates to the read side flaps'
// list handler queries (Corrosion). launchLookupDelay gives it a brief
// window to catch up; in the unlikely event a duplicate does slip through,
// it will be tagged with the same bg.timestamp as the tracked one and get
// swept by the normal blue→green cycle on the next deployment. That's a
// strictly better outcome than aborting the deployment mid-flight.
func (bg *blueGreen) launchGreenMachineWithRetry(ctx context.Context, launchInput *fly.LaunchMachineInput, launchID string) (*fly.Machine, error) {
	var lastErr error
	attempts := bg.launchRetryAttempts
	if attempts == 0 {
		attempts = 1
	}

	delay := bg.launchRetryDelay
	for attempt := uint(0); attempt < attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		newMachineRaw, err := bg.flaps.Launch(ctx, bg.app.Name, *launchInput)
		if err == nil {
			return newMachineRaw, nil
		}
		lastErr = err

		// A non-transient failure (400/404/etc.) isn't going to get better
		// with another attempt.
		if !isTransientFlapsError(err) {
			return nil, err
		}

		// This might have been a silent success. Give flaps' backing store a
		// moment to reflect the commit, then look for a machine with our
		// launch ID.
		if bg.launchLookupDelay > 0 {
			select {
			case <-time.After(bg.launchLookupDelay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		if existing, lookupErr := bg.findGreenMachineByLaunchID(ctx, launchID); lookupErr != nil {
			// A failed lookup shouldn't mask the original Launch error, but we
			// still want the user to see it so they can debug if the retry
			// also fails.
			fmt.Fprintf(bg.io.ErrOut, "  Idempotency lookup after failed launch returned an error: %v\n", lookupErr)
		} else if existing != nil {
			fmt.Fprintf(bg.io.ErrOut,
				"  Launch reported an error but machine %s was already created (idempotency tag matched); reusing it\n",
				bg.colorize.Bold(existing.ID))

			return existing, nil
		}

		// Not the last attempt: back off and try again.
		if attempt+1 < attempts {
			fmt.Fprintf(bg.io.ErrOut, "  Retrying green machine launch (attempt %d/%d): %v\n",
				attempt+2, attempts, err)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			if delay < 5*time.Second {
				delay *= 2
			}
		}
	}

	return nil, lastErr
}

// findGreenMachineByLaunchID scans the app's machines and returns the one
// whose metadata carries the given launch ID, if any. It's a client-side
// filter because flaps' List does not (yet) expose a metadata query in the
// fly-go client interface.
func (bg *blueGreen) findGreenMachineByLaunchID(ctx context.Context, launchID string) (*fly.Machine, error) {
	machines, err := bg.flaps.List(ctx, bg.app.Name, "")
	if err != nil {
		return nil, err
	}

	for _, m := range machines {
		if m == nil || m.Config == nil {
			continue
		}
		if m.Config.Metadata[flyctlBGLaunchIDMetadataKey] == launchID {
			return m, nil
		}
	}

	return nil, nil
}

func hasTransientNetworkSubstring(err error) bool {
	message := strings.ToLower(err.Error())
	for _, s := range []string{
		"connection reset by peer",
		"connection refused",
		"network is unreachable",
		"temporary failure in name resolution",
		"no such host",
		"eof",
	} {
		if strings.Contains(message, s) {
			return true
		}
	}

	return false
}
