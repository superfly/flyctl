package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/recipe"
	"github.com/superfly/flyctl/internal/flag"
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

	template := recipe.RecipeTemplate{
		Name:         "Rolling restart",
		RequireLease: true,
		Operations: []recipe.Operation{
			{
				Name: "restart",
				Type: recipe.OperationTypeMachine,
				MachineCommand: recipe.MachineCommand{
					Action: "restart",
				},
				HealthCheckSelector: recipe.HealthCheckSelector{
					Name:  "role",
					Value: "replica",
				},
			},
			{
				Name: "failover",
				Type: recipe.OperationTypeHTTP,
				HTTPCommand: recipe.HTTPCommand{
					Method:   "GET",
					Endpoint: "/commands/admin/failover/trigger",
					Port:     5500,
				},
				HealthCheckSelector: recipe.HealthCheckSelector{
					Name:  "role",
					Value: "leader",
				},
			},
			{
				Name: "restart",
				Type: recipe.OperationTypeMachine,
				MachineCommand: recipe.MachineCommand{
					Action: "restart",
				},
				HealthCheckSelector: recipe.HealthCheckSelector{
					Name:  "role",
					Value: "leader",
				},
			},
		},
	}

	template.Process(ctx)

	return nil
}

func nomadRestart(ctx context.Context, allocs []*api.AllocationStatus) (err error) {
	var (
		dialer   = agent.DialerFromContext(ctx)
		client   = client.FromContext(ctx).API()
		appName  = app.NameFromContext(ctx)
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
	)

	leader, replicas, err := nomadNodeRoles(ctx, allocs)
	if err != nil {
		return
	}

	if leader == nil {
		return fmt.Errorf("no leader found")
	}

	if len(replicas) > 0 {
		fmt.Fprintln(io.Out, "Attempting to restart replica(s)")

		for _, replica := range replicas {
			fmt.Fprintf(io.Out, " Restarting %s\n", replica.ID)

			if err := client.RestartAllocation(ctx, appName, replica.ID); err != nil {
				return fmt.Errorf("failed to restart vm %s: %w", replica.ID, err)
			}
			// wait for health checks to pass
		}
	}

	// Don't perform failover if the cluster is only running a
	// single node.
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

	if err := client.RestartAllocation(ctx, appName, leader.ID); err != nil {
		return fmt.Errorf("failed to restart vm %s: %w", leader.ID, err)
	}

	fmt.Fprintf(io.Out, "Postgres cluster has been successfully restarted!\n")

	return
}
