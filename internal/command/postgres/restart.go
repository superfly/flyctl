package postgres

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
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
			Description: "Force a restart even we don't have an active leader",
			Default:     false,
		},
		flag.Bool{
			Name:        "skip-health-checks",
			Description: "Runs rolling restart process without waiting for health checks. ( Machines only )",
			Default:     false,
		},
	)

	return cmd
}

func runRestart(ctx context.Context) error {
	var (
		appName = app.NameFromContext(ctx)
		client  = client.FromContext(ctx).API()
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	if !app.IsPostgresApp() {
		return fmt.Errorf("app %s is not a postgres app", appName)
	}

	ctx, err = apps.BuildContext(ctx, app)
	if err != nil {
		return err
	}

	switch app.PlatformVersion {
	case "machines":
		input := api.RestartMachineInput{
			SkipHealthChecks: flag.GetBool(ctx, "skip-health-checks"),
		}

		return machinesRestart(ctx, &input)
	case "nomad":
		return nomadRestart(ctx, app)
	default:
		return fmt.Errorf("unknown platform version")
	}
}

func machinesRestart(ctx context.Context, input *api.RestartMachineInput) (err error) {
	var (
		MinPostgresHaVersion         = "0.0.20"
		MinPostgresFlexVersion       = "0.0.3"
		MinPostgresStandaloneVersion = "0.0.7"

		dialer   = agent.DialerFromContext(ctx)
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()

		force = flag.GetBool(ctx, "force")
	)

	machines, releaseLeaseFunc, err := mach.AcquireAllLeases(ctx)
	defer releaseLeaseFunc(ctx, machines)
	if err != nil {
		return err
	}

	_, dev := os.LookupEnv("FLY_DEV")
	if !dev {
		if err := hasRequiredVersionOnMachines(machines, MinPostgresHaVersion, MinPostgresFlexVersion, MinPostgresStandaloneVersion); err != nil {
			return err
		}
	}

	leader, replicas := machinesNodeRoles(ctx, machines)
	if leader == nil {
		if !force {
			return fmt.Errorf("no active leader found")
		}
		fmt.Fprintln(io.Out, colorize.Yellow("No leader found, but continuing with restart"))
	}

	manager := flypg.StolonManager
	if leader.ImageRef.Repository == "flyio/postgres-flex" {
		manager = flypg.ReplicationManager
	}

	fmt.Fprintln(io.Out, "Identifying cluster role(s)")
	for _, machine := range machines {
		fmt.Fprintf(io.Out, "  Machine %s: %s\n", colorize.Bold(machine.ID), machineRole(machine))
	}

	// Restarting replicas
	for _, replica := range replicas {
		if err = mach.Restart(ctx, replica, input); err != nil {
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

	if inRegionReplicas > 0 && manager != flypg.ReplicationManager {
		pgclient := flypg.NewFromInstance(leader.PrivateIP, dialer)
		fmt.Fprintf(io.Out, "Attempting to failover %s\n", colorize.Bold(leader.ID))

		if err := pgclient.Failover(ctx); err != nil {
			msg := fmt.Sprintf("failed to perform failover: %s", err.Error())
			if !force {
				return fmt.Errorf(msg)
			}

			fmt.Fprintln(io.Out, colorize.Red(msg))
		}
	}

	if err = mach.Restart(ctx, leader, input); err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Postgres cluster has been successfully restarted!\n")

	return
}

func nomadRestart(ctx context.Context, app *api.AppCompact) error {
	var (
		MinPostgresHaVersion = "0.0.20"

		client   = client.FromContext(ctx).API()
		dialer   = agent.DialerFromContext(ctx)
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
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

	if err := client.RestartAllocation(ctx, app.Name, leader.ID); err != nil {
		return fmt.Errorf("failed to restart vm %s: %w", leader.ID, err)
	}

	fmt.Fprintf(io.Out, "Postgres cluster has been successfully restarted!\n")

	return nil

}
