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
	"github.com/superfly/flyctl/internal/flapsutil"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

func newRestart() *cobra.Command {
	const (
		short = "Restart a Fly Postgres cluster one machine at a time"
		long  = `Restart every machine in a Fly Postgres cluster, replicas first and the
primary last. The Postgres app comes from --app or fly.toml. There's no
confirmation prompt.

On Stolon clusters with a replica in the primary's region, the primary
role is moved to a replica before the old primary restarts. The command
stops if the cluster has no primary, unless you pass --force.`
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
	)

	app, err := flapsutil.ClientFromContext(ctx).GetApp(ctx, appName)
	if err != nil {
		return err
	}

	if !flapsutil.IsPostgresApp(app) {
		return fmt.Errorf("app %s is not a postgres app", appName)
	}

	ctx, err = apps.BuildContextForApp(ctx, app)
	if err != nil {
		return err
	}

	input := fly.RestartMachineInput{
		SkipHealthChecks: flag.GetBool(ctx, "skip-health-checks"),
	}

	return machinesRestart(ctx, appName, &input, flag.GetBool(ctx, "force"))
}

// machinesRestart restarts every member of the cluster, replicas first. force
// lets the restart continue without an active leader and past a failed
// failover; callers pass it explicitly because other commands define their own
// unrelated --force flag.
func machinesRestart(ctx context.Context, appName string, input *fly.RestartMachineInput, force bool) (err error) {
	var (
		MinPostgresHaVersion         = "0.0.20"
		MinPostgresFlexVersion       = "0.0.3"
		MinPostgresStandaloneVersion = "0.0.7"

		dialer   = agent.DialerFromContext(ctx)
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
	)

	machines, releaseLeaseFunc, err := mach.AcquireAllLeases(ctx, appName)
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
		if err = mach.Restart(ctx, appName, replica, input, replica.LeaseNonce); err != nil {
			return err
		}
	}

	// Don't attempt to failover unless we have in-region replicas
	inRegionReplicas := 0
	if leader != nil {
		for _, replica := range replicas {
			if replica.Region == leader.Region {
				inRegionReplicas++
			}
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

	// There is no leader to restart when --force continued without one.
	if leader == nil {
		fmt.Fprintln(io.Out, colorize.Yellow(fmt.Sprintf(
			"Restarted %d replica(s), but the cluster still has no active leader. Check its health with `fly checks list`.",
			len(replicas),
		)))
		return
	}

	if err = mach.Restart(ctx, appName, leader, input, leader.LeaseNonce); err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Postgres cluster has been successfully restarted!\n")

	return
}
