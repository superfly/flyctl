package machine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jpillora/backoff"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/ctrlc"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/statuslogger"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/maps"
)

type LeasableMachine interface {
	Machine() *fly.Machine
	HasLease() bool
	AcquireLease(context.Context, time.Duration) error
	RefreshLease(context.Context, time.Duration) error
	ReleaseLease(context.Context) error
	StartBackgroundLeaseRefresh(context.Context, time.Duration, time.Duration)
	Update(context.Context, fly.LaunchMachineInput) error
	Start(context.Context) error
	Stop(context.Context, string) error
	Destroy(context.Context, bool) error
	Cordon(context.Context) error
	WaitForState(context.Context, string, time.Duration, bool) error
	WaitForSmokeChecksToPass(context.Context) error
	WaitForHealthchecksToPass(context.Context, time.Duration) error
	WaitForEventType(context.Context, string, time.Duration, bool) (*fly.MachineEvent, error)
	WaitForEventTypeAfterType(context.Context, string, string, time.Duration, bool) (*fly.MachineEvent, error)
	FormattedMachineId() string
	GetMinIntervalAndMinGracePeriod() (time.Duration, time.Duration)
	SetMetadata(ctx context.Context, k, v string) error
	GetMetadata(ctx context.Context) (map[string]string, error)
}

type leasableMachine struct {
	flapsClient            flapsutil.FlapsClient
	io                     *iostreams.IOStreams
	colorize               *iostreams.ColorScheme
	machine                *fly.Machine
	leaseRefreshCancelFunc context.CancelFunc
	destroyed              bool
	showLogs               bool

	// mu protects leaseNonce. A leasableMachine shouldn't be shared between
	// goroutines, but StartBackgroundLeaseRefresh breaks the rule.
	mu         sync.Mutex
	leaseNonce string
}

// NewLeasableMachine creates a wrapper for the given machine.
// A lease must be held before calling this function.
func NewLeasableMachine(flapsClient flapsutil.FlapsClient, io *iostreams.IOStreams, machine *fly.Machine, showLogs bool) LeasableMachine {
	// TODO: make sure the other functions handle showLogs correctly
	return &leasableMachine{
		flapsClient: flapsClient,
		io:          io,
		colorize:    io.ColorScheme(),
		machine:     machine,
		leaseNonce:  machine.LeaseNonce,
		showLogs:    showLogs,
	}
}

func (lm *leasableMachine) Update(ctx context.Context, input fly.LaunchMachineInput) error {
	if lm.IsDestroyed() {
		return fmt.Errorf("error cannot update machine %s that was already destroyed", lm.machine.ID)
	}
	if !lm.HasLease() {
		return fmt.Errorf("no current lease for machine %s", lm.machine.ID)
	}
	input.ID = lm.machine.ID
	updateMachine, err := lm.flapsClient.Update(ctx, input, lm.leaseNonce)
	if err != nil {
		return err
	}
	lm.machine = updateMachine
	return nil
}

func (lm *leasableMachine) Stop(ctx context.Context, signal string) error {
	if lm.IsDestroyed() {
		return fmt.Errorf("cannon stop machine %s that was already destroyed", lm.machine.ID)
	}

	input := fly.StopMachineInput{
		ID:     lm.machine.ID,
		Signal: signal,
	}

	return lm.flapsClient.Stop(ctx, input, lm.leaseNonce)
}

func (lm *leasableMachine) Destroy(ctx context.Context, kill bool) error {
	if lm.IsDestroyed() {
		return nil
	}
	input := fly.RemoveMachineInput{
		ID:   lm.machine.ID,
		Kill: kill,
	}
	err := lm.flapsClient.Destroy(ctx, input, lm.leaseNonce)
	if err != nil {
		return err
	}
	lm.destroyed = true
	return nil
}

func (lm *leasableMachine) Cordon(ctx context.Context) error {
	if lm.IsDestroyed() {
		return fmt.Errorf("cannon cordon machine %s that was already destroyed", lm.machine.ID)
	}

	return lm.flapsClient.Cordon(ctx, lm.machine.ID, lm.leaseNonce)
}

func (lm *leasableMachine) FormattedMachineId() string {
	m := lm.Machine()
	processGroup := m.ProcessGroup()
	if processGroup == "" || m.IsFlyAppsReleaseCommand() || m.IsFlyAppsConsole() {
		return m.ID
	}
	return fmt.Sprintf("%s [%s]", m.ID, processGroup)
}

func (lm *leasableMachine) logStatusWaiting(ctx context.Context, desired string) {
	statuslogger.Logf(ctx, "Waiting for %s to have state: %s", lm.colorize.Bold(lm.FormattedMachineId()), lm.colorize.Yellow(desired))
}

func (lm *leasableMachine) logStatusFinished(ctx context.Context, current string) {
	statuslogger.Logf(ctx, "Machine %s has state: %s", lm.colorize.Bold(lm.FormattedMachineId()), lm.colorize.Green(current))
}

func (lm *leasableMachine) logHealthCheckStatus(ctx context.Context, status *fly.HealthCheckStatus) {
	if status == nil {
		return
	}
	resColor := lm.colorize.Green
	if status.Passing != status.Total {
		resColor = lm.colorize.Yellow
	}
	statuslogger.Logf(ctx,
		"Waiting for %s to become healthy: %s\n",
		lm.colorize.Bold(lm.FormattedMachineId()),
		resColor(fmt.Sprintf("%d/%d", status.Passing, status.Total)),
	)
}

func (lm *leasableMachine) Start(ctx context.Context) error {
	if lm.IsDestroyed() {
		return fmt.Errorf("error cannot start machine %s that was already destroyed", lm.machine.ID)
	}
	if lm.HasLease() {
		return fmt.Errorf("error cannot start machine %s because it has a lease", lm.machine.ID)
	}
	if lm.showLogs {
		lm.logStatusWaiting(ctx, fly.MachineStateStarted)
	}
	_, err := lm.flapsClient.Start(ctx, lm.machine.ID, "")
	if err != nil {
		return err
	}
	return nil
}

// resolveTimeoutContext returns a context that will timeout, with the possibility of an untimed context on time=0 if
// the flag allowInfinite is set to true.
func resolveTimeoutContext(ctx context.Context, timeout time.Duration, allowInfinite bool) (context.Context, context.CancelFunc, time.Duration) {
	// This situation is a bug, so we'll just go with vaguely normal behavior.
	if !allowInfinite && timeout == 0 {
		terminal.Warnf("resolveTimeoutContext: allowInfinite is false and timeout is 0, setting timeout to 2 minutes\n")
		timeout = 2 * time.Minute
	}
	if timeout != 0 {
		// If we have a timeout, put it on the context.
		waitCtx, cancel := context.WithTimeout(ctx, timeout)
		return waitCtx, cancel, timeout
	} else {
		// We'll set a timeout of 2 minutes for flaps, knowing that it will keep
		// polling until the machine is in the desired state. (this is just so we don't keep spamming the API)
		// We'll keep polling because the actual stopping mechanism is the context, which we haven't set a timeout on.
		return ctx, func() {}, 2 * time.Minute
	}
}

func (lm *leasableMachine) WaitForState(ctx context.Context, desiredState string, timeout time.Duration, allowInfinite bool) error {
	waitCtx, cancel, timeout := resolveTimeoutContext(ctx, timeout, allowInfinite)
	waitCtx, cancel = ctrlc.HookCancelableContext(waitCtx, cancel)
	defer cancel()
	b := &backoff.Backoff{
		Min:    500 * time.Millisecond,
		Max:    2 * time.Second,
		Factor: 2,
		Jitter: true,
	}
	if lm.showLogs {
		lm.logStatusWaiting(ctx, desiredState)
	}
	for {
		err := lm.flapsClient.Wait(waitCtx, lm.Machine(), desiredState, timeout)
		notFoundResponse := false
		if err != nil {
			var flapsErr *flaps.FlapsError
			if errors.As(err, &flapsErr) {
				notFoundResponse = flapsErr.ResponseStatusCode == http.StatusNotFound
			}
		}
		switch {
		case errors.Is(waitCtx.Err(), context.Canceled):
			return err
		case errors.Is(waitCtx.Err(), context.DeadlineExceeded):
			return WaitTimeoutErr{
				machineID:    lm.machine.ID,
				timeout:      timeout,
				desiredState: desiredState,
			}
		case notFoundResponse && desiredState != fly.MachineStateDestroyed:
			return err
		case !notFoundResponse && err != nil:
			select {
			case <-time.After(b.Duration()):
			case <-waitCtx.Done():
			}
			continue
		}
		if lm.showLogs {
			lm.logStatusFinished(ctx, desiredState)
		}
		return nil
	}
}

func (lm *leasableMachine) isConstantlyRestarting(machine *fly.Machine) bool {
	var ev *fly.MachineEvent

	for _, mev := range machine.Events {
		if mev.Type == "exit" {
			ev = mev
			break
		}
	}

	if ev == nil {
		return false
	}

	return !ev.Request.ExitEvent.RequestedStop &&
		ev.Request.ExitEvent.Restarting &&
		ev.Request.RestartCount > 1 &&
		ev.Request.ExitEvent.ExitCode != 0
}

func (lm *leasableMachine) WaitForSmokeChecksToPass(ctx context.Context) error {
	ctx, span := tracing.GetTracer().Start(ctx, "wait_for_smoke_checks")
	defer span.End()

	waitCtx, cancel := ctrlc.HookCancelableContext(context.WithTimeout(ctx, 10*time.Second))
	defer cancel()

	b := &backoff.Backoff{
		Min:    500 * time.Millisecond,
		Max:    2 * time.Second,
		Factor: 2,
		Jitter: true,
	}

	if lm.showLogs {
		statuslogger.Logf(ctx, "Checking that %s is up and running", lm.colorize.Bold(lm.FormattedMachineId()))
	}

	for {
		machine, err := lm.flapsClient.Get(waitCtx, lm.Machine().ID)
		startedAt, startedAtErr := machine.MostRecentStartTimeAfterLaunch()
		uptime := 0 * time.Second
		if startedAtErr == nil {
			uptime = time.Since(startedAt)
		}
		switch {
		case uptime > 10*time.Second && !lm.isConstantlyRestarting(machine):
			return nil
		case errors.Is(waitCtx.Err(), context.Canceled):
			return err
		case errors.Is(waitCtx.Err(), context.DeadlineExceeded):
			return nil
		case err != nil:
			span.RecordError(err)
			return fmt.Errorf("error getting machine %s from api: %w", lm.Machine().ID, err)
		}

		switch {
		case lm.isConstantlyRestarting(machine):
			err := fmt.Errorf("the app appears to be crashing")
			span.RecordError(err)
			return err
		default:
			select {
			case <-time.After(b.Duration()):
			case <-waitCtx.Done():
			}
		}
	}
}

func (lm *leasableMachine) WaitForHealthchecksToPass(ctx context.Context, timeout time.Duration) error {
	ctx, span := tracing.GetTracer().Start(ctx, "wait_for_healthchecks", trace.WithAttributes(attribute.Int("num_checks", len(lm.Machine().Checks)), attribute.Int64("timeout_ms", timeout.Milliseconds())))
	defer span.End()
	if len(lm.Machine().Checks) == 0 {
		return nil
	}
	waitCtx, cancel := ctrlc.HookCancelableContext(context.WithTimeout(ctx, timeout))
	defer cancel()

	checkDefs := maps.Values(lm.Machine().Config.Checks)
	for _, s := range lm.Machine().Config.Services {
		checkDefs = append(checkDefs, s.Checks...)
	}
	shortestInterval := 120 * time.Second
	for _, c := range checkDefs {
		if c.Interval != nil && c.Interval.Duration < shortestInterval {
			shortestInterval = c.Interval.Duration
		}
	}
	b := &backoff.Backoff{
		Min:    1 * time.Second,
		Max:    2 * time.Second,
		Jitter: true,
	}

	printedFirst := false
	for {
		updateMachine, err := lm.flapsClient.Get(waitCtx, lm.Machine().ID)
		switch {
		case errors.Is(waitCtx.Err(), context.Canceled):
			span.RecordError(err)
			return err
		case errors.Is(waitCtx.Err(), context.DeadlineExceeded):
			span.RecordError(err)
			return fmt.Errorf("timeout reached waiting for health checks to pass for machine %s: %w", lm.Machine().ID, err)
		case err != nil:
			span.RecordError(err)
			return fmt.Errorf("error getting machine %s from api: %w", lm.Machine().ID, err)
		case !updateMachine.AllHealthChecks().AllPassing():
			if lm.showLogs && (!printedFirst || lm.io.IsInteractive()) {
				lm.logHealthCheckStatus(ctx, updateMachine.AllHealthChecks())
				printedFirst = true
			}
			select {
			case <-time.After(b.Duration()):
			case <-waitCtx.Done():
			}
			continue
		}
		if lm.showLogs {
			lm.logHealthCheckStatus(ctx, updateMachine.AllHealthChecks())
		}
		return nil
	}
}

func (lm *leasableMachine) WaitForEventType(ctx context.Context, eventType string, timeout time.Duration, allowInfinite bool) (*fly.MachineEvent, error) {
	waitCtx, cancel, _ := resolveTimeoutContext(ctx, timeout, allowInfinite)
	waitCtx, cancel = ctrlc.HookCancelableContext(waitCtx, cancel)
	defer cancel()
	b := &backoff.Backoff{
		Min:    500 * time.Millisecond,
		Max:    2 * time.Second,
		Factor: 2,
		Jitter: true,
	}
	if lm.showLogs {
		statuslogger.Logf(ctx, "Waiting for %s to get %s event", lm.colorize.Bold(lm.FormattedMachineId()), lm.colorize.Yellow(eventType))
	}
	for {
		updateMachine, err := lm.flapsClient.Get(waitCtx, lm.Machine().ID)
		switch {
		case errors.Is(waitCtx.Err(), context.Canceled):
			return nil, err
		case errors.Is(waitCtx.Err(), context.DeadlineExceeded):
			return nil, fmt.Errorf("timeout reached waiting for health checks to pass for machine %s: %w", lm.Machine().ID, err)
		case err != nil:
			return nil, fmt.Errorf("error getting machine %s from api: %w", lm.Machine().ID, err)
		}

		exitEvent := updateMachine.GetLatestEventOfType(eventType)
		if exitEvent != nil {
			return exitEvent, nil
		} else {
			select {
			case <-time.After(b.Duration()):
			case <-waitCtx.Done():
			}
		}
	}
}

// waits for an eventType1 type event to show up after we see a eventType2 event, and returns it
func (lm *leasableMachine) WaitForEventTypeAfterType(ctx context.Context, eventType1, eventType2 string, timeout time.Duration, allowInfinite bool) (*fly.MachineEvent, error) {
	waitCtx, cancel, _ := resolveTimeoutContext(ctx, timeout, allowInfinite)
	waitCtx, cancel = ctrlc.HookCancelableContext(waitCtx, cancel)
	defer cancel()
	b := &backoff.Backoff{
		Min:    500 * time.Millisecond,
		Max:    2 * time.Second,
		Factor: 2,
		Jitter: true,
	}
	statuslogger.Logf(ctx, "Waiting for %s to get %s event", lm.colorize.Bold(lm.FormattedMachineId()), lm.colorize.Yellow(eventType1))
	for {
		updateMachine, err := lm.flapsClient.Get(waitCtx, lm.Machine().ID)
		switch {
		case errors.Is(waitCtx.Err(), context.Canceled):
			return nil, err
		case errors.Is(waitCtx.Err(), context.DeadlineExceeded):
			return nil, fmt.Errorf("timeout reached waiting for health checks to pass for machine %s: %w", lm.Machine().ID, err)
		case err != nil:
			return nil, fmt.Errorf("error getting machine %s from api: %w", lm.Machine().ID, err)
		}
		exitEvent := updateMachine.GetLatestEventOfTypeAfterType(eventType1, eventType2)
		if exitEvent != nil {
			return exitEvent, nil
		} else {
			select {
			case <-time.After(b.Duration()):
			case <-waitCtx.Done():
			}
		}
	}
}

func (lm *leasableMachine) Machine() *fly.Machine {
	return lm.machine
}

func (lm *leasableMachine) HasLease() bool {
	return lm.leaseNonce != ""
}

func (lm *leasableMachine) IsDestroyed() bool {
	return lm.destroyed
}

func (lm *leasableMachine) AcquireLease(ctx context.Context, duration time.Duration) error {
	if lm.HasLease() {
		return nil
	}
	seconds := int(duration.Seconds())
	lease, err := lm.flapsClient.AcquireLease(ctx, lm.machine.ID, &seconds)
	if err != nil {
		return err
	}
	if lease.Status != "success" {
		return fmt.Errorf("did not acquire lease for machine %s status: %s code: %s message: %s", lm.machine.ID, lease.Status, lease.Code, lease.Message)
	}
	if lease.Data == nil {
		return fmt.Errorf("missing data from lease response for machine %s, assuming not successful", lm.machine.ID)
	}
	terminal.Debugf("got lease on machine %s: %v\n", lm.machine.ID, lease)
	lm.leaseNonce = lease.Data.Nonce
	return nil
}

func (lm *leasableMachine) RefreshLease(ctx context.Context, duration time.Duration) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	seconds := int(duration.Seconds())
	refreshedLease, err := lm.flapsClient.RefreshLease(ctx, lm.machine.ID, &seconds, lm.leaseNonce)
	if err != nil {
		return err
	}
	if refreshedLease.Status != "success" {
		return fmt.Errorf("did not acquire lease for machine %s status: %s code: %s message: %s", lm.machine.ID, refreshedLease.Status, refreshedLease.Code, refreshedLease.Message)
	} else if refreshedLease.Data == nil {
		return fmt.Errorf("missing data from lease response for machine %s, assuming not successful", lm.machine.ID)
	} else if refreshedLease.Data.Nonce != lm.leaseNonce {
		return fmt.Errorf("unexpectedly received a new nonce when trying to refresh lease on machine %s", lm.machine.ID)
	}
	terminal.Debugf("got lease on machine %s: %v\n", lm.machine.ID, refreshedLease)
	return nil
}

func (lm *leasableMachine) StartBackgroundLeaseRefresh(ctx context.Context, leaseDuration time.Duration, delayBetween time.Duration) {
	ctx, lm.leaseRefreshCancelFunc = context.WithCancel(ctx)
	go lm.refreshLeaseUntilCanceled(ctx, leaseDuration, delayBetween)
}

func (lm *leasableMachine) refreshLeaseUntilCanceled(ctx context.Context, duration time.Duration, delayBetween time.Duration) {
	b := &backoff.Backoff{
		Min:    delayBetween - 20*time.Millisecond,
		Max:    delayBetween + 20*time.Millisecond,
		Jitter: true,
	}

	for {
		time.Sleep(b.Duration())
		switch err := lm.RefreshLease(ctx, duration); {
		case err == nil:
			// good times
		case errors.Is(err, context.Canceled):
			return
		case strings.Contains(err.Error(), "machine not found"):
			// machine is gone, no need to refresh its lease
			return
		default:
			terminal.Warnf("error refreshing lease for machine %s: %v\n", lm.machine.ID, err)
		}
	}
}

// ReleaseLease releases the lease on this machine.
func (lm *leasableMachine) ReleaseLease(ctx context.Context) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	nonce := lm.leaseNonce
	lm.resetLease()
	if nonce == "" {
		return nil
	}

	// when context is canceled, take 500ms to attempt to release the leases
	contextWasAlreadyCanceled := errors.Is(ctx.Err(), context.Canceled)
	if contextWasAlreadyCanceled {
		var cancel context.CancelFunc
		cancelTimeout := 500 * time.Millisecond
		ctx, cancel = context.WithTimeout(ctx, cancelTimeout)
		terminal.Infof("detected canceled context and allowing %s to release machine %s lease\n", cancelTimeout, lm.FormattedMachineId())
		defer cancel()
	}

	return lm.flapsClient.ReleaseLease(ctx, lm.machine.ID, nonce)
}

func (lm *leasableMachine) resetLease() {
	lm.leaseNonce = ""
	if lm.leaseRefreshCancelFunc != nil {
		lm.leaseRefreshCancelFunc()
	}
}

func (lm *leasableMachine) GetMinIntervalAndMinGracePeriod() (time.Duration, time.Duration) {
	minInterval := 60 * time.Second

	checkDefs := maps.Values(lm.Machine().Config.Checks)
	for _, s := range lm.Machine().Config.Services {
		checkDefs = append(checkDefs, s.Checks...)
	}
	minGracePeriod := time.Second
	for _, c := range checkDefs {
		if c.Interval != nil && c.Interval.Duration < minInterval {
			minInterval = c.Interval.Duration
		}

		if c.GracePeriod != nil && c.GracePeriod.Duration < minGracePeriod {
			minGracePeriod = c.GracePeriod.Duration
		}
	}

	return minInterval, minGracePeriod
}

func (lm *leasableMachine) GetMetadata(ctx context.Context) (map[string]string, error) {
	return lm.flapsClient.GetMetadata(ctx, lm.machine.ID)
}

func (lm *leasableMachine) SetMetadata(ctx context.Context, k, v string) error {
	return lm.flapsClient.SetMetadata(ctx, lm.machine.ID, k, v)
}
