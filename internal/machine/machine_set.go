package machine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/exp/slices"
)

type MachineSet interface {
	AcquireLeases(context.Context, time.Duration) error
	ReleaseLeases(context.Context) error
	RemoveMachines(ctx context.Context, machines []LeasableMachine) error
	StartBackgroundLeaseRefresh(context.Context, time.Duration, time.Duration)
	IsEmpty() bool
	GetMachines() []LeasableMachine
}

type machineSet struct {
	machines []LeasableMachine
}

func NewMachineSet(flapsClient *flaps.Client, io *iostreams.IOStreams, machines []*api.Machine) *machineSet {
	leaseMachines := make([]LeasableMachine, 0)
	for _, m := range machines {
		leaseMachines = append(leaseMachines, NewLeasableMachine(flapsClient, io, m))
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

func (ms *machineSet) AcquireLeases(ctx context.Context, duration time.Duration) error {
	if len(ms.machines) == 0 {
		return nil
	}

	results := make(chan error, len(ms.machines))
	var wg sync.WaitGroup
	for _, m := range ms.machines {
		wg.Add(1)
		go func(m LeasableMachine) {
			defer wg.Done()
			results <- m.AcquireLease(ctx, duration)
		}(m)
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	hadError := false
	for err := range results {
		if err != nil {
			hadError = true
			terminal.Warnf("failed to acquire lease: %v\n", err)
		}
	}
	if hadError {
		if err := ms.ReleaseLeases(ctx); err != nil {
			terminal.Warnf("error releasing machine leases: %v\n", err)
		}
		return fmt.Errorf("error acquiring leases on all machines")
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

func (ms *machineSet) ReleaseLeases(ctx context.Context) error {
	if len(ms.machines) == 0 {
		return nil
	}

	// when context is canceled, take 500ms to attempt to release the leases
	contextWasAlreadyCanceled := errors.Is(ctx.Err(), context.Canceled)
	if contextWasAlreadyCanceled {
		var cancel context.CancelFunc
		cancelTimeout := 500 * time.Millisecond
		ctx, cancel = context.WithTimeout(context.TODO(), cancelTimeout)
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
		contextTimedOutOrCanceled := errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
		if err != nil && (!contextWasAlreadyCanceled || !contextTimedOutOrCanceled || !strings.Contains(err.Error(), "lease not found")) {
			hadError = true
			terminal.Warnf("failed to release lease: %v\n", err)
		}
	}
	if hadError {
		return fmt.Errorf("error releasing leases on machines")
	}
	return nil
}

func (ms *machineSet) StartBackgroundLeaseRefresh(ctx context.Context, leaseDuration time.Duration, delayBetween time.Duration) {
	for _, m := range ms.machines {
		m.StartBackgroundLeaseRefresh(ctx, leaseDuration, delayBetween)
	}
}
