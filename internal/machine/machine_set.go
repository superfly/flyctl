package machine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/sync/errgroup"
)

type MachineSet interface {
	AcquireLeases(context.Context, time.Duration) error
	ReleaseLeases(context.Context) error
	RemoveMachines(ctx context.Context, machines []LeasableMachine) error
	StartBackgroundLeaseRefresh(context.Context, time.Duration, time.Duration)
	IsEmpty() bool
	GetMachines() []LeasableMachine
	WaitForMachineSetState(context.Context, string, time.Duration, bool, bool) ([]string, error)
}

type machineSet struct {
	machines []LeasableMachine
}

func NewMachineSet(flapsClient flapsutil.FlapsClient, io *iostreams.IOStreams, machines []*fly.Machine, showLogs bool) *machineSet {
	leaseMachines := make([]LeasableMachine, 0)
	for _, m := range machines {
		leaseMachines = append(leaseMachines, NewLeasableMachine(flapsClient, io, m, showLogs))
	}
	return &machineSet{
		machines: leaseMachines,
	}
}

func (ms *machineSet) IsEmpty() bool {
	return len(ms.machines) == 0
}

func (ms *machineSet) GetMachines() []LeasableMachine {
	return ms.machines
}

// AcquireLeases acquires leases on all machines in the set for the given duration.
func (ms *machineSet) AcquireLeases(ctx context.Context, duration time.Duration) error {
	if len(ms.machines) == 0 {
		return nil
	}

	// Don't override ctx. Even leaseCtx is cancelled, we still want to release the leases.
	eg, leaseCtx := errgroup.WithContext(ctx)
	for _, m := range ms.machines {
		eg.Go(func() error {
			return m.AcquireLease(leaseCtx, duration)
		})
	}

	waitErr := eg.Wait()
	if waitErr != nil {
		terminal.Warnf("failed to acquire lease: %v\n", waitErr)
		if err := ms.ReleaseLeases(ctx); err != nil {
			terminal.Warnf("error releasing machine leases: %v\n", err)
		}
		return waitErr
	}
	return nil
}

func (ms *machineSet) RemoveMachines(ctx context.Context, machines []LeasableMachine) error {
	// Rewrite machines array to exclude the ones we just released.
	tempMachines := ms.machines[:0]

	// Compute the intersection between all of the machines on machineSet with the machines we want to remove.
	for _, oldMach := range ms.machines {
		if !slices.ContainsFunc(machines, func(m LeasableMachine) bool { return oldMach.Machine().ID == m.Machine().ID }) {
			tempMachines = append(tempMachines, oldMach)
		}
	}

	ms.machines = tempMachines

	subset := machineSet{machines: machines}
	return subset.ReleaseLeases(ctx)
}

// ReleaseLeases releases leases on all machines in this set.
func (ms *machineSet) ReleaseLeases(ctx context.Context) error {
	if len(ms.machines) == 0 {
		return nil
	}

	// when context is canceled, take 500ms to attempt to release the leases
	contextWasAlreadyCanceled := errors.Is(ctx.Err(), context.Canceled)
	if contextWasAlreadyCanceled {
		var cancel context.CancelFunc
		cancelTimeout := 500 * time.Millisecond
		ctx, cancel = context.WithTimeout(ctx, cancelTimeout)
		terminal.Infof("detected canceled context and allowing %s to release machine leases\n", cancelTimeout)
		defer cancel()
	}

	results := make(chan error, len(ms.machines))
	var wg sync.WaitGroup
	for _, m := range ms.machines {
		wg.Add(1)
		go func(m LeasableMachine) {
			defer wg.Done()
			results <- m.ReleaseLease(ctx)
		}(m)
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	hadError := false
	for err := range results {
		if err != nil {
			contextTimedOutOrCanceled := errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
			var ferr *flaps.FlapsError
			if errors.As(err, &ferr) && ferr.ResponseStatusCode == http.StatusNotFound {
				// Having StatusNotFound is expected, if acquiring this entire set is partially failing.
			} else if !contextWasAlreadyCanceled || !contextTimedOutOrCanceled {
				hadError = true
				terminal.Warnf("failed to release lease: %v\n", err)
			}
		}
	}
	if hadError {
		return fmt.Errorf("error releasing leases on machines")
	}
	return nil
}

func (ms *machineSet) StartBackgroundLeaseRefresh(ctx context.Context, leaseDuration time.Duration, delayBetween time.Duration) {
	ctx, span := tracing.GetTracer().Start(ctx, "start_background_lease_refresh")
	defer span.End()

	for _, m := range ms.machines {
		m.StartBackgroundLeaseRefresh(ctx, leaseDuration, delayBetween)
	}
}

func (ms *machineSet) WaitForMachineSetState(ctx context.Context, state string, timeout time.Duration, allowInfinite, allowNotFound bool) ([]string, error) {
	if len(ms.machines) == 0 {
		return nil, nil
	}

	terminal.Debug("waiting for test machine state ", state)

	results := make(chan error, len(ms.machines))
	var wg sync.WaitGroup
	for _, m := range ms.machines {
		wg.Add(1)
		go func(m LeasableMachine) {
			defer wg.Done()
			err := m.WaitForState(ctx, state, timeout, allowInfinite)

			var flapsErr *flaps.FlapsError
			if errors.As(err, &flapsErr) {
				if flapsErr.ResponseStatusCode == http.StatusNotFound && allowNotFound {
					results <- nil
					return
				}
			}

			results <- err
		}(m)
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	badMachineIDs := make([]string, 0)
	hadError := false
	for err := range results {
		if err != nil {
			hadError = true
			terminal.Warnf("failed to wait for state: %v\n", err)
			badMachineIDs = append(badMachineIDs, err.Error())
		}
	}
	if hadError {
		return badMachineIDs, fmt.Errorf("error waiting for state on all machines")
	}
	return nil, nil
}
