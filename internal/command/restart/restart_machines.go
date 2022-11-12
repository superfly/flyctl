package restart

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/command/machine"
	"github.com/superfly/flyctl/internal/command/postgres"
	"github.com/superfly/flyctl/internal/watch"
)

func runMachineRestart(ctx context.Context, app *api.AppCompact) error {
	flapsClient := flaps.FromContext(ctx)

	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return err
	}

	// Acquire leases
	for _, machine := range machines {
		lease, err := flapsClient.GetLease(ctx, machine.ID, api.IntPointer(40))
		if err != nil {
			return fmt.Errorf("failed to obtain lease: %w", err)
		}
		machine.LeaseNonce = lease.Data.Nonce

		// Ensure lease is released on return
		defer flapsClient.ReleaseLease(ctx, machine.ID, machine.LeaseNonce)
	}

	machines, err = flapsClient.ListActive(ctx)
	if err != nil {
		return err
	}

	if app.PostgresAppRole != nil && app.PostgresAppRole.Name == "postgres_cluster" {
		return postgres.RestartMachines(ctx, machines)
	}

	return runMachineBaseRestart(ctx, machines)
}

func runMachineBaseRestart(ctx context.Context, machines []*api.Machine) error {
	for _, m := range machines {
		if err := machine.Restart(ctx, m.ID, "", 120, false); err != nil {
			return err
		}
		// wait for health checks to pass
		if err := watch.MachinesChecks(ctx, []*api.Machine{m}); err != nil {
			return fmt.Errorf("failed to wait for health checks to pass: %w", err)
		}
	}

	return nil
}
