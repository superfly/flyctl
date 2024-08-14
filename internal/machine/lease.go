package machine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sourcegraph/conc/pool"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/iostreams"
)

const maxConcurrentLeases = 20

type releaseLeaseFunc func()

// AcquireAllLeases works to acquire/attach a lease for each active machine.
func AcquireAllLeases(ctx context.Context) ([]*fly.Machine, releaseLeaseFunc, error) {
	machines, err := ListActive(ctx)
	if err != nil {
		return nil, func() {}, err
	}

	return AcquireLeases(ctx, machines)
}

// AcquireLeases works to acquire/attach a lease for each machine specified.
func AcquireLeases(ctx context.Context, machines []*fly.Machine) ([]*fly.Machine, releaseLeaseFunc, error) {
	acquirePool := pool.NewWithResults[*fly.Machine]().
		WithErrors().
		WithMaxGoroutines(maxConcurrentLeases)

	for _, m := range machines {
		m := m
		acquirePool.Go(func() (*fly.Machine, error) {
			// Skip leasing for unreachable machines
			if m.HostStatus != fly.HostStatusOk {
				return m, nil
			}

			m, _, err := AcquireLease(ctx, m)
			return m, err
		})
	}

	leaseHoldingMachines, err := acquirePool.Wait()

	releaseFunc := func() {
		p := pool.New()
		for _, m := range leaseHoldingMachines {
			p.Go(func() { releaseLease(ctx, m) })
		}
		p.Wait()
	}

	return leaseHoldingMachines, releaseFunc, err
}

func releaseLease(ctx context.Context, machine *fly.Machine) {
	if machine == nil || machine.LeaseNonce == "" {
		return
	}

	io := iostreams.FromContext(ctx)
	flapsClient := flapsutil.ClientFromContext(ctx)

	// remove the cancel from ctx so we can still releases leases if the command was aborted
	if ctx.Err() != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()

		fmt.Fprintf(io.Out, "Releasing lease for machine %s...\n", machine.ID)
	}

	if err := flapsClient.ReleaseLease(ctx, machine.ID, machine.LeaseNonce); err != nil {
		if !strings.Contains(err.Error(), "lease not found") {
			fmt.Fprintf(io.Out, "failed to release lease for machine %s: %s\n", machine.ID, err.Error())
		}
	}
}

// AcquireLease works to acquire/attach a lease for the specified machine.
// WARNING: Make sure you defer the lease release process.
func AcquireLease(ctx context.Context, machine *fly.Machine) (*fly.Machine, releaseLeaseFunc, error) {
	// if we haven't gotten the lease after 2s, we print a message so users
	// aren't left wondering.
	abortStatusUpdate := make(chan struct{})
	defer close(abortStatusUpdate)
	go func() {
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()

		select {
		case <-timer.C:
			io := iostreams.FromContext(ctx)
			fmt.Fprintf(io.Out, "Waiting on lease for machine %s...\n", machine.ID)
		case <-abortStatusUpdate:
		}
	}()

	flapsClient := flapsutil.ClientFromContext(ctx)

	lease, err := flapsClient.AcquireLease(ctx, machine.ID, fly.IntPointer(120))
	if err != nil {
		return nil, func() {}, fmt.Errorf("failed to obtain lease: %w", err)
	}
	releaseFunc := func() { releaseLease(ctx, machine) }

	// Set lease nonce before we re-fetch the Machines latest configuration.
	// This will ensure the lease can still be released in the event the upcoming GET fails.
	machine.LeaseNonce = lease.Data.Nonce

	// Return earlier if the lease's machine version matches the machine's version we have
	if machine.InstanceID == lease.Data.Version {
		return machine, releaseFunc, nil
	}

	// Re-query machine post-lease acquisition to ensure we are working against the latest configuration.
	updatedMachine, err := flapsClient.Get(ctx, machine.ID)
	if err != nil {
		return machine, releaseFunc, err
	}

	updatedMachine.LeaseNonce = lease.Data.Nonce
	releaseFunc = func() { releaseLease(ctx, updatedMachine) }
	return updatedMachine, releaseFunc, nil
}
