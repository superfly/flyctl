package machine

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
)

// AcquireLease acquire leases for all active Machines.
// WARNING: Make sure you defer the lease release process.
func AcquireLeases(ctx context.Context) ([]*api.Machine, error) {
	var (
		flapsClient = flaps.FromContext(ctx)
	)

	machines, err := ListActive(ctx)
	if err != nil {
		return nil, err
	}

	// Acquire leases
	for _, machine := range machines {
		lease, err := flapsClient.GetLease(ctx, machine.ID, api.IntPointer(40))
		if err != nil {
			return nil, fmt.Errorf("failed to obtain lease: %w", err)
		}
		machine.LeaseNonce = lease.Data.Nonce
	}

	// Requery machines to ensure we are working against the most up-to-date configuration.
	machines, err = ListActive(ctx)
	if err != nil {
		return nil, err
	}

	return machines, nil
}
