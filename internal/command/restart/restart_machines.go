package restart

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/command/machine"
	"github.com/superfly/flyctl/internal/command/postgres"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"
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

	if app.PostgresAppRole.Name == "postgres_cluster" {
		return runMachinePGRestart(ctx, machines)
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

func runMachinePGRestart(ctx context.Context, machines []*api.Machine) (err error) {
	var (
		io                   = iostreams.FromContext(ctx)
		colorize             = io.ColorScheme()
		dialer               = agent.DialerFromContext(ctx)
		MinPostgresHaVersion = "0.0.20"
	)

	if err := postgres.MachinePGVersionCompatible(machines, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
		return err
	}

	leader, replicas := postgres.MachinesNodeRoles(ctx, machines)

	// unless flag.force is set, we should error if leader==nil
	if flag.GetBool(ctx, "force") && leader == nil {
		fmt.Fprintln(io.Out, colorize.Yellow("No leader found, but continuing with restart"))
	} else if leader == nil {
		return fmt.Errorf("no active leader found")
	}

	fmt.Fprintln(io.Out, "Identifying cluster role(s)")

	for _, machine := range machines {
		fmt.Fprintf(io.Out, "  Machine %s: %s\n", colorize.Bold(machine.ID), postgres.MachineRole(machine))
	}

	if len(replicas) > 0 {
		for _, replica := range replicas {
			fmt.Fprintf(io.Out, "Restarting machine %s\n", colorize.Bold(replica.ID))

			if err = machine.Restart(ctx, replica.ID, "", 120, false); err != nil {
				return err
			}
			// wait for health checks to pass
			if err := watch.MachinesChecks(ctx, []*api.Machine{replica}); err != nil {
				return fmt.Errorf("failed to wait for health checks to pass: %w", err)
			}
		}
	}

	// Don't attempt to failover unless we have in-region replicas
	inRegionReplicas := 0
	for _, replica := range replicas {
		if replica.Region == leader.Region {
			inRegionReplicas++
		}
	}

	if inRegionReplicas > 0 {
		pgclient := flypg.NewFromInstance(leader.PrivateIP, dialer)

		fmt.Fprintf(io.Out, "Attempting to failover %s\n", colorize.Bold(leader.ID))
		if err := pgclient.Failover(ctx); err != nil {
			fmt.Fprintln(io.Out, colorize.Red(fmt.Sprintf("failed to perform failover: %s", err.Error())))
		}
	}

	fmt.Fprintf(io.Out, "Restarting machine %s\n", colorize.Bold(leader.ID))

	if err = machine.Restart(ctx, leader.ID, "", 120, false); err != nil {
		return err
	}
	// wait for health checks to pass
	if err := watch.MachinesChecks(ctx, []*api.Machine{leader}); err != nil {
		return fmt.Errorf("failed to wait for health checks to pass: %w", err)
	}

	fmt.Fprintf(io.Out, "Postgres cluster has been successfully restarted!\n")

	return
}
