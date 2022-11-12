package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/machine"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"
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

func runRestart(ctx context.Context) error {
	return fmt.Errorf("this command has been removed. Use `flyctl restart` instead")
}

func RestartMachines(ctx context.Context, machines []*api.Machine) (err error) {
	var (
		io                   = iostreams.FromContext(ctx)
		colorize             = io.ColorScheme()
		dialer               = agent.DialerFromContext(ctx)
		MinPostgresHaVersion = "0.0.20"
	)

	if err := hasRequiredVersionOnMachines(machines, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
		return err
	}

	leader, replicas := machinesNodeRoles(ctx, machines)

	// unless flag.force is set, we should error if leader==nil
	if flag.GetBool(ctx, "force") && leader == nil {
		fmt.Fprintln(io.Out, colorize.Yellow("No leader found, but continuing with restart"))
	} else if leader == nil {
		return fmt.Errorf("no active leader found")
	}

	fmt.Fprintln(io.Out, "Identifying cluster role(s)")

	for _, machine := range machines {
		fmt.Fprintf(io.Out, "  Machine %s: %s\n", colorize.Bold(machine.ID), machineRole(machine))
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

func RestartNomad(ctx context.Context, app *api.AppCompact) error {
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
