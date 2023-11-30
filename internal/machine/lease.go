package machine

import (
	"context"
	"fmt"
	"strings"

	"github.com/sourcegraph/conc/pool"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/iostreams"
)

const maxConcurrentLeases = 20

type releaseLeaseFunc func()

// AcquireAllLeases works to acquire/attach a lease for each active machine.
func AcquireAllLeases(ctx context.Context) ([]*api.Machine, releaseLeaseFunc, error) {
	machines, err := ListActive(ctx)
	if err != nil {
		return nil, func() {}, err
	}

	return AcquireLeases(ctx, machines)
}

// AcquireLeases works to acquire/attach a lease for each machine specified.
func AcquireLeases(ctx context.Context, machines []*api.Machine) ([]*api.Machine, releaseLeaseFunc, error) {
	acquirePool := pool.NewWithResults[*api.Machine]().
		WithErrors().
		WithMaxGoroutines(maxConcurrentLeases)

	for _, m := range machines {
		m := m
		acquirePool.Go(func() (*api.Machine, error) {
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

func releaseLease(ctx context.Context, machine *api.Machine) {
	if machine == nil || machine.LeaseNonce == "" {
		return
	}

	io := iostreams.FromContext(ctx)
	flapsClient := flaps.FromContext(ctx)

	if err := flapsClient.ReleaseLease(ctx, machine.ID, machine.LeaseNonce); err != nil {
		if !strings.Contains(err.Error(), "lease not found") {
			fmt.Fprintf(io.Out, "failed to release lease for machine %s: %s", machine.ID, err.Error())
		}
	}
}

// AcquireLease works to acquire/attach a lease for the specified machine.
// WARNING: Make sure you defer the lease release process.
func AcquireLease(ctx context.Context, machine *api.Machine) (*api.Machine, releaseLeaseFunc, error) {
	flapsClient := flaps.FromContext(ctx)

	lease, err := flapsClient.AcquireLease(ctx, machine.ID, api.IntPointer(120))
	if err != nil {
		return nil, func() {}, fmt.Errorf("failed to obtain lease: %w", err)
	}

	// Set lease nonce before we re-fetch the Machines latest configuration.
	// This will ensure the lease can still be released in the event the upcoming GET fails.
	machine.LeaseNonce = lease.Data.Nonce

	// Re-query machine post-lease acquisition to ensure we are working against the latest configuration.
	updatedMachine, err := flapsClient.Get(ctx, machine.ID)
	if err != nil {
		return machine, func() { releaseLease(ctx, machine) }, err
	}

	updatedMachine.LeaseNonce = lease.Data.Nonce
	return updatedMachine, func() { releaseLease(ctx, updatedMachine) }, nil
}
