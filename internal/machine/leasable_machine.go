package machine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jpillora/backoff"
	"github.com/morikuni/aec"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/exp/maps"
)

type LeasableMachine interface {
	Machine() *api.Machine
	HasLease() bool
	AcquireLease(context.Context, time.Duration) error
	RefreshLease(context.Context, time.Duration) error
	ReleaseLease(context.Context) error
	StartBackgroundLeaseRefresh(context.Context, time.Duration, time.Duration)
	Update(context.Context, api.LaunchMachineInput) error
	Start(context.Context) error
	Destroy(context.Context, bool) error
	WaitForState(context.Context, string, time.Duration, string) error
	WaitForHealthchecksToPass(context.Context, time.Duration, string) error
	WaitForEventTypeAfterType(context.Context, string, string, time.Duration) (*api.MachineEvent, error)
	FormattedMachineId() string
}

type leasableMachine struct {
	flapsClient            *flaps.Client
	io                     *iostreams.IOStreams
	colorize               *iostreams.ColorScheme
	machine                *api.Machine
	leaseNonce             string
	leaseRefreshCancelFunc context.CancelFunc
	destroyed              bool
}

func NewLeasableMachine(flapsClient *flaps.Client, io *iostreams.IOStreams, machine *api.Machine) LeasableMachine {
	return &leasableMachine{
		flapsClient: flapsClient,
		io:          io,
		colorize:    io.ColorScheme(),
		machine:     machine,
		leaseNonce:  machine.LeaseNonce,
	}
}

func (lm *leasableMachine) Update(ctx context.Context, input api.LaunchMachineInput) error {
	if lm.IsDestroyed() {
		return fmt.Errorf("error cannot update machine %s that was already destroyed", lm.machine.ID)
	}
	if !lm.HasLease() {
		return fmt.Errorf("no current lease for machine %s", lm.machine.ID)
	}
	updateMachine, err := lm.flapsClient.Update(ctx, input, lm.leaseNonce)
	if err != nil {
		return err
	}
	lm.machine = updateMachine
	return nil
}

func (lm *leasableMachine) Destroy(ctx context.Context, kill bool) error {
	if lm.IsDestroyed() {
		return nil
	}
	input := api.RemoveMachineInput{
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

func (lm *leasableMachine) FormattedMachineId() string {
	res := lm.Machine().ID
	if lm.Machine().Config.Metadata == nil {
		return res
	}
	procGroup := lm.Machine().ProcessGroup()
	if procGroup == "" || lm.Machine().IsFlyAppsReleaseCommand() {
		return res
	}
	return fmt.Sprintf("%s [%s]", res, procGroup)
}

func (lm *leasableMachine) logClearLinesAbove(count int) {
	if lm.io.IsInteractive() {
		builder := aec.EmptyBuilder
		str := builder.Up(uint(count)).EraseLine(aec.EraseModes.All).ANSI
		fmt.Fprint(lm.io.ErrOut, str.String())
	}
}

func (lm *leasableMachine) logStatusWaiting(desired, prefix string) {
	if prefix != "" {
		prefix += " "
	}
	fmt.Fprintf(lm.io.ErrOut, "  %sWaiting for %s to have state: %s\n",
		prefix,
		lm.colorize.Bold(lm.FormattedMachineId()),
		lm.colorize.Yellow(desired),
	)
}

func (lm *leasableMachine) logStatusFinished(current string) {
	fmt.Fprintf(lm.io.ErrOut, "  Machine %s has state: %s\n",
		lm.colorize.Bold(lm.FormattedMachineId()),
		lm.colorize.Green(current),
	)
}

func (lm *leasableMachine) logHealthCheckStatus(status *api.HealthCheckStatus, prefix string) {
	if status == nil {
		return
	}
	resColor := lm.colorize.Green
	if status.Passing != status.Total {
		resColor = lm.colorize.Yellow
	}
	if prefix != "" {
		prefix += " "
	}
	fmt.Fprintf(lm.io.ErrOut, "  %sWaiting for %s to become healthy: %s\n",
		prefix,
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
	lm.logStatusWaiting(api.MachineStateStarted, "")
	_, err := lm.flapsClient.Start(ctx, lm.machine.ID)
	if err != nil {
		return err
	}
	return nil
}

func (lm *leasableMachine) WaitForState(ctx context.Context, desiredState string, timeout time.Duration, logPrefix string) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	b := &backoff.Backoff{
		Min:    500 * time.Millisecond,
		Max:    2 * time.Second,
		Factor: 2,
		Jitter: true,
	}
	lm.logClearLinesAbove(1)
	lm.logStatusWaiting(desiredState, logPrefix)
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
			return fmt.Errorf("timeout reached waiting for machine to %s %w", desiredState, err)
		case notFoundResponse && desiredState != api.MachineStateDestroyed:
			return err
		case !notFoundResponse && err != nil:
			time.Sleep(b.Duration())
			continue
		}
		lm.logClearLinesAbove(1)
		lm.logStatusFinished(desiredState)
		return nil
	}
}

func (lm *leasableMachine) WaitForHealthchecksToPass(ctx context.Context, timeout time.Duration, logPrefix string) error {
	if len(lm.Machine().Checks) == 0 {
		return nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
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
		Min:    shortestInterval / 2,
		Max:    2 * shortestInterval,
		Factor: 2,
		Jitter: true,
	}

	printedFirst := false
	for {
		updateMachine, err := lm.flapsClient.Get(waitCtx, lm.Machine().ID)
		switch {
		case errors.Is(waitCtx.Err(), context.Canceled):
			return err
		case errors.Is(waitCtx.Err(), context.DeadlineExceeded):
			return fmt.Errorf("timeout reached waiting for healthchecks to pass for machine %s %w", lm.Machine().ID, err)
		case err != nil:
			return fmt.Errorf("error getting machine %s from api: %w", lm.Machine().ID, err)
		case !updateMachine.HealthCheckStatus().AllPassing():
			if !printedFirst || lm.io.IsInteractive() {
				lm.logClearLinesAbove(1)
				lm.logHealthCheckStatus(updateMachine.HealthCheckStatus(), logPrefix)
				printedFirst = true
			}
			time.Sleep(b.Duration())
			continue
		}
		lm.logClearLinesAbove(1)
		lm.logHealthCheckStatus(updateMachine.HealthCheckStatus(), logPrefix)
		return nil
	}
}

// waits for an eventType1 type event to show up after we see a eventType2 event, and returns it
func (lm *leasableMachine) WaitForEventTypeAfterType(ctx context.Context, eventType1, eventType2 string, timeout time.Duration) (*api.MachineEvent, error) {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	b := &backoff.Backoff{
		Min:    500 * time.Millisecond,
		Max:    2 * time.Second,
		Factor: 2,
		Jitter: true,
	}
	lm.logClearLinesAbove(1)
	fmt.Fprintf(lm.io.ErrOut, "  Waiting for %s to get %s event\n",
		lm.colorize.Bold(lm.FormattedMachineId()),
		lm.colorize.Yellow(eventType1),
	)
	for {
		updateMachine, err := lm.flapsClient.Get(waitCtx, lm.Machine().ID)
		switch {
		case errors.Is(waitCtx.Err(), context.Canceled):
			return nil, err
		case errors.Is(waitCtx.Err(), context.DeadlineExceeded):
			return nil, fmt.Errorf("timeout reached waiting for healthchecks to pass for machine %s %w", lm.Machine().ID, err)
		case err != nil:
			return nil, fmt.Errorf("error getting machine %s from api: %w", lm.Machine().ID, err)
		}
		exitEvent := updateMachine.GetLatestEventOfTypeAfterType(eventType1, eventType2)
		if exitEvent != nil {
			return exitEvent, nil
		} else {
			time.Sleep(b.Duration())
		}
	}
}

func (lm *leasableMachine) Machine() *api.Machine {
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
	lm.leaseNonce = lease.Data.Nonce
	return nil
}

func (lm *leasableMachine) RefreshLease(ctx context.Context, duration time.Duration) error {
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
	return nil
}

func (lm *leasableMachine) StartBackgroundLeaseRefresh(ctx context.Context, leaseDuration time.Duration, delayBetween time.Duration) {
	ctx, lm.leaseRefreshCancelFunc = context.WithCancel(ctx)
	go lm.refreshLeaseUntilCanceled(ctx, leaseDuration, delayBetween)
}

func (lm *leasableMachine) refreshLeaseUntilCanceled(ctx context.Context, duration time.Duration, delayBetween time.Duration) {
	var (
		err error
		b   = &backoff.Backoff{
			Min:    delayBetween - 20*time.Millisecond,
			Max:    delayBetween + 20*time.Millisecond,
			Jitter: true,
		}
	)
	for {
		err = lm.RefreshLease(ctx, duration)
		switch {
		case errors.Is(err, context.Canceled):
			return
		case err != nil:
			terminal.Warnf("error refreshing lease for machine %s: %v\n", lm.machine.ID, err)
		}
		time.Sleep(b.Duration())
	}
}

func (lm *leasableMachine) ReleaseLease(ctx context.Context) error {
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
		ctx, cancel = context.WithTimeout(context.TODO(), cancelTimeout)
		terminal.Infof("detected canceled context and allowing %s to release machine %s lease\n", cancelTimeout, lm.FormattedMachineId())
		defer cancel()
	}

	err := lm.flapsClient.ReleaseLease(ctx, lm.machine.ID, nonce)
	contextTimedOutOrCanceled := errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
	if err != nil && (!contextWasAlreadyCanceled || !contextTimedOutOrCanceled) {
		terminal.Warnf("failed to release lease for machine %s: %v\n", lm.machine.ID, err)
		return err
	}
	return nil
}

func (lm *leasableMachine) resetLease() {
	lm.leaseNonce = ""
	if lm.leaseRefreshCancelFunc != nil {
		lm.leaseRefreshCancelFunc()
	}
}
