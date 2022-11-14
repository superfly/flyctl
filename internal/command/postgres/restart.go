package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/machine"
)

func newRestart() *cobra.Command {
	const (
		short = "Restarts each member of the Postgres cluster one by one."
		long  = short + " Downtime should be minimal." + "\n"
		usage = "restart"
	)

	cmd := command.New(usage, short, long, runRestart,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Bool{
			Name:        "force",
			Shorthand:   "f",
			Description: "Force a restart even we don't have an active leader",
			Default:     false,
		},
	)

	return cmd
}

// The cli aspects of this file can be removed after December 1st 2022
func runRestart(ctx context.Context) error {
	return fmt.Errorf("this command has been removed. Use `flyctl restart` instead")
}

// Machine specific restart logic.
func MachinesRestart(ctx context.Context) (err error) {
	var (
		io                   = iostreams.FromContext(ctx)
		colorize             = io.ColorScheme()
		dialer               = agent.DialerFromContext(ctx)
		flapsClient          = flaps.FromContext(ctx)
		MinPostgresHaVersion = "0.0.20"
	)

	force := flag.GetBool(ctx, "force")

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

	// Requery machines to ensure we are working against the most up-to-date configuration.
	machines, err = flapsClient.ListActive(ctx)
	if err != nil {
		return err
	}

	if err := hasRequiredVersionOnMachines(machines, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
		return err
	}

	leader, replicas := machinesNodeRoles(ctx, machines)

	if leader == nil && force {
		fmt.Fprintln(io.Out, colorize.Yellow("No leader found, but continuing with restart"))
	} else {
		return fmt.Errorf("no active leader found")
	}

	fmt.Fprintln(io.Out, "Identifying cluster role(s)")
	for _, machine := range machines {
		fmt.Fprintf(io.Out, "  Machine %s: %s\n", colorize.Bold(machine.ID), machineRole(machine))
	}

	// Restarting replicas
	for _, replica := range replicas {
		if err = machine.Restart(ctx, replica); err != nil {
			return err
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

	if err = machine.Restart(ctx, leader); err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Postgres cluster has been successfully restarted!\n")

	return
}

// Nomad specific restart logic
func NomadRestart(ctx context.Context, app *api.AppCompact) error {
	var (
		dialer               = agent.DialerFromContext(ctx)
		client               = client.FromContext(ctx).API()
		io                   = iostreams.FromContext(ctx)
		colorize             = io.ColorScheme()
		MinPostgresHaVersion = "0.0.20"
	)

	if err := hasRequiredVersionOnNomad(app, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
		return err
	}

	allocs, err := client.GetAllocations(ctx, app.Name, false)
	if err != nil {
		return fmt.Errorf("can't fetch allocations: %w", err)
	}

	leader, replicas, err := nomadNodeRoles(ctx, allocs)
	if err != nil {
		return err
	}

	if leader == nil {
		return fmt.Errorf("no leader found")
	}

	if len(replicas) > 0 {
		fmt.Fprintln(io.Out, "Attempting to restart replica(s)")

		for _, replica := range replicas {
			fmt.Fprintf(io.Out, " Restarting %s\n", replica.ID)

			if err := client.RestartAllocation(ctx, app.Name, replica.ID); err != nil {
				return fmt.Errorf("failed to restart vm %s: %w", replica.ID, err)
			}
			// TODO - wait for health checks to pass
		}
	}

	// Don't perform failover if the cluster is only running a single node.
	if len(allocs) > 1 {
		pgclient := flypg.NewFromInstance(leader.PrivateIP, dialer)

		fmt.Fprintf(io.Out, "Performing a failover\n")
		if err := pgclient.Failover(ctx); err != nil {
			if err := pgclient.Failover(ctx); err != nil {
				fmt.Fprintln(io.Out, colorize.Yellow(fmt.Sprintf("WARN: failed to perform failover: %s", err.Error())))
			}
		}
	}

	fmt.Fprintln(io.Out, "Attempting to restart leader")

	if err := client.RestartAllocation(ctx, app.Name, leader.ID); err != nil {
		return fmt.Errorf("failed to restart vm %s: %w", leader.ID, err)
	}

	fmt.Fprintf(io.Out, "Postgres cluster has been successfully restarted!\n")

	return nil

}
