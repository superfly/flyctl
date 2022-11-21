package machine

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/iostreams"
)

type ReleaseLeasesFunc func(ctx context.Context, machines []*api.Machine)
type ReleaseLeaseFunc func(ctx context.Context, machine *api.Machine)

// AcquireAllLeases works to acquire/attach a lease for each active machine.
func AcquireAllLeases(ctx context.Context) ([]*api.Machine, ReleaseLeasesFunc, error) {
	releaseFunc := func(ctx context.Context, machines []*api.Machine) { return }

	machines, err := ListActive(ctx)
	if err != nil {
		return nil, releaseFunc, err
	}

	return AcquireLeases(ctx, machines)
}

// AcquireLeases works to acquire/attach a lease for each machine specified.
func AcquireLeases(ctx context.Context, machines []*api.Machine) ([]*api.Machine, ReleaseLeasesFunc, error) {
	var (
		flapsClient = flaps.FromContext(ctx)
		io          = iostreams.FromContext(ctx)
	)

	releaseFunc := func(ctx context.Context, machines []*api.Machine) {
		for _, m := range machines {
			if err := flapsClient.ReleaseLease(ctx, m.ID, m.LeaseNonce); err != nil {
				fmt.Fprintf(io.Out, "failed to release lease for machine %s: %s", m.ID, err.Error())
			}
		}
	}

	leaseHoldingMachines := []*api.Machine{}
	for _, machine := range machines {
		m, _, err := AcquireLease(ctx, machine)
		if err != nil {
			return leaseHoldingMachines, releaseFunc, err
		}
		leaseHoldingMachines = append(leaseHoldingMachines, m)
	}

	return leaseHoldingMachines, releaseFunc, nil
}

// AcquireLease works to acquire/attach a lease for the specified machine.
// WARNING: Make sure you defer the lease release process.
func AcquireLease(ctx context.Context, machine *api.Machine) (*api.Machine, ReleaseLeaseFunc, error) {
	var (
		flapsClient = flaps.FromContext(ctx)
	)

	lease, err := flapsClient.AcquireLease(ctx, machine.ID, api.IntPointer(40))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to obtain lease: %w", err)
	}

	machine, err = flapsClient.Get(ctx, machine.ID)
	if err != nil {
		return nil, nil, err
	}

	machine.LeaseNonce = lease.Data.Nonce

	releaseFunc := func(ctx context.Context, machine *api.Machine) {
		flapsClient := flaps.FromContext(ctx)
		flapsClient.ReleaseLease(ctx, machine.ID, machine.LeaseNonce)
	}

	return machine, releaseFunc, nil
}
