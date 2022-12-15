package machine

import (
	"context"
	"fmt"
	"strings"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/iostreams"
)

type releaseLeasesFunc func(ctx context.Context, machines []*api.Machine)
type releaseLeaseFunc func(ctx context.Context, machine *api.Machine)

// AcquireAllLeases works to acquire/attach a lease for each active machine.
func AcquireAllLeases(ctx context.Context) ([]*api.Machine, releaseLeasesFunc, error) {
	releaseFunc := func(ctx context.Context, machines []*api.Machine) {}

	machines, err := ListActive(ctx)
	if err != nil {
		return nil, releaseFunc, err
	}

	return AcquireLeases(ctx, machines)
}

// AcquireLeases works to acquire/attach a lease for each machine specified.
func AcquireLeases(ctx context.Context, machines []*api.Machine) ([]*api.Machine, releaseLeasesFunc, error) {
	var (
		flapsClient = flaps.FromContext(ctx)
		io          = iostreams.FromContext(ctx)
	)

	releaseFunc := func(ctx context.Context, machines []*api.Machine) {
		for _, m := range machines {
			if err := flapsClient.ReleaseLease(ctx, m.ID, m.LeaseNonce); err != nil {
				if !strings.Contains(err.Error(), "lease not found") {
					fmt.Fprintf(io.Out, "failed to release lease for machine %s: %s", m.ID, err.Error())
				}
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
func AcquireLease(ctx context.Context, machine *api.Machine) (*api.Machine, releaseLeaseFunc, error) {
	var (
		flapsClient = flaps.FromContext(ctx)
		io          = iostreams.FromContext(ctx)
	)

	releaseFunc := func(ctx context.Context, machine *api.Machine) {
		if machine != nil {
			if err := flapsClient.ReleaseLease(ctx, machine.ID, machine.LeaseNonce); err != nil {
				fmt.Fprintf(io.Out, "failed to release lease for machine %s: %s\n", machine.ID, err.Error())
			}
		}
	}

	lease, err := flapsClient.AcquireLease(ctx, machine.ID, api.IntPointer(120))
	if err != nil {
		return nil, releaseFunc, fmt.Errorf("failed to obtain lease: %w", err)
	}

	// Set lease nonce before we re-fetch the Machines latest configuration.
	// This will ensure the lease can still be released in the event the upcoming GET fails.
	machine.LeaseNonce = lease.Data.Nonce

	// Re-query machine post-lease acquisition to ensure we are working against the latest configuration.
	machine, err = flapsClient.Get(ctx, machine.ID)
	if err != nil {
		return machine, releaseFunc, err
	}

	machine.LeaseNonce = lease.Data.Nonce

	return machine, releaseFunc, nil
}
