package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
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
			Description: "Runs rolling restart process without waiting for health checks.",
			Default:     false,
		},
	)

	return cmd
}

func runRestart(ctx context.Context) error {
	var (
		appName = appconfig.NameFromContext(ctx)
		client  = flyutil.ClientFromContext(ctx)
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

	input := fly.RestartMachineInput{
		SkipHealthChecks: flag.GetBool(ctx, "skip-health-checks"),
	}
	return machinesRestart(ctx, appName, &input)
}

func machinesRestart(ctx context.Context, appName string, input *fly.RestartMachineInput) (err error) {
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
	defer releaseLeaseFunc()
	if err != nil {
		return err
	}

	if err := hasRequiredVersionOnMachines(appName, machines, MinPostgresHaVersion, MinPostgresFlexVersion, MinPostgresStandaloneVersion); err != nil {
		return err
	}

	leader, replicas := machinesNodeRoles(ctx, machines)
	if leader == nil {
		if !force {
			return fmt.Errorf("no active leader found")
		}
		fmt.Fprintln(io.Out, colorize.Yellow("No leader found, but continuing with restart"))
	}

	manager := flypg.StolonManager
	if IsFlex(leader) {
		manager = flypg.ReplicationManager
	}

	fmt.Fprintln(io.Out, "Identifying cluster role(s)")
	for _, machine := range machines {
		fmt.Fprintf(io.Out, "  Machine %s: %s\n", colorize.Bold(machine.ID), machineRole(machine))
	}

	// Restarting replicas
	for _, replica := range replicas {
		if err = mach.Restart(ctx, replica, input, replica.LeaseNonce); err != nil {
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
				return fmt.Errorf("failed to perform failover: %w", err)
			}

			fmt.Fprintln(io.Out, colorize.Red(msg))
		}
	}

	if err = mach.Restart(ctx, leader, input, leader.LeaseNonce); err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Postgres cluster has been successfully restarted!\n")

	return
}
