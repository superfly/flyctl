package machine

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
)

// AcquireAllLeases works to acquire/attach a lease for each active machine.
// WARNING: Make sure you defer the lease release process.
func AcquireAllLeases(ctx context.Context) ([]*api.Machine, error) {
	machines, err := ListActive(ctx)
	if err != nil {
		return nil, err
	}

	return AcquireLeases(ctx, machines)

}

// AcquireLeases works to acquire/attach a lease for each machine specified.
// WARNING: Make sure you defer the lease release process.
func AcquireLeases(ctx context.Context, machines []*api.Machine) ([]*api.Machine, error) {
	var leaseHoldingMachines []*api.Machine

	for _, machine := range machines {
		m, err := AcquireLease(ctx, machine)
		if err != nil {
			return nil, err
		}
		leaseHoldingMachines = append(leaseHoldingMachines, m)
	}

	return leaseHoldingMachines, nil
}

// AcquireLease works to acquire/attach a lease for the specified machine.
// WARNING: Make sure you defer the lease release process.
func AcquireLease(ctx context.Context, machine *api.Machine) (*api.Machine, error) {
	var (
		flapsClient = flaps.FromContext(ctx)
	)

	lease, err := flapsClient.AcquireLease(ctx, machine.ID, api.IntPointer(40))
	if err != nil {
		return nil, fmt.Errorf("failed to obtain lease: %w", err)
	}

	machine, err = flapsClient.Get(ctx, machine.ID)
	if err != nil {
		return nil, err
	}

	machine.LeaseNonce = lease.Data.Nonce

	return machine, nil
}
