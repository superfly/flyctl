package postgres

import (
	"context"
	"fmt"
	"net/http"

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

// imageVersionStr := app.ImageDetails.Version[1:]
// imageVersion, err := version.NewVersion(imageVersionStr)
// if err != nil {
// 	return err
// }

// // Specify compatible versions per repo.
// requiredVersion := &version.Version{}
// if app.ImageDetails.Repository == "flyio/postgres-standalone" {
// 	requiredVersion, err = version.NewVersion(standalone)
// 	if err != nil {
// 		return err
// 	}
// }
// if app.ImageDetails.Repository == "flyio/postgres" {
// 	requiredVersion, err = version.NewVersion(cluster)
// 	if err != nil {
// 		return err
// 	}
// }

// if requiredVersion == nil {
// 	return fmt.Errorf("unable to resolve image version")
// }

// if imageVersion.LessThan(requiredVersion) {
// 	return fmt.Errorf(
// 		"image version is not compatible. (Current: %s, Required: >= %s)\n"+
// 			"Please run 'flyctl image show' and update to the latest available version",
// 		imageVersion, requiredVersion.String())
// }

// return nil

func runRestart(ctx context.Context) error {
	var (
		MinPostgresHaVersion = "0.0.20"
		client               = client.FromContext(ctx).API()
		appName              = app.NameFromContext(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	switch app.PlatformVersion {
	case "nomad":
		if err = hasRequiredVersionOnNomad(app, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
			return err
		}
		vms, err := client.GetAllocations(ctx, app.Name, false)
		if err != nil {
			return fmt.Errorf("can't fetch allocations: %w", err)
		}
		return nomadRestart(ctx, vms)
	case "machines":

		template := recipe.RecipeTemplate{
			Name:         "Rolling restart",
			App:          app,
			RequireLease: true,
			Constraints: recipe.Constraints{
				AppRoleID: "postgres_cluster",
				Images: []recipe.ImageRequirements{
					{
						Repository:    "flyio/postgres",
						MinFlyVersion: "0.0.20",
					},
					{
						Repository: "flyio/postgres-standalone",
					},
				},
			},
			Operations: []*recipe.Operation{
				{
					Name: "restart",
					Type: recipe.CommandTypeFlaps,
					FlapsCommand: recipe.FlapsCommand{
						Action: "restart",
						Method: http.MethodPost,
						Options: map[string]string{
							"force_stop": "true",
						},
					},
					Selector: recipe.Selector{
						HealthCheck: recipe.HealthCheckSelector{
							Name:  "role",
							Value: "replica",
						},
					},
					WaitForHealthChecks: true,
				},
				{
					Name: "failover",
					Type: recipe.CommandTypeHTTP,
					HTTPCommand: recipe.HTTPCommand{
						Method:   "GET",
						Endpoint: "/commands/admin/failover/trigger",
						Port:     5500,
						Result:   new(recipe.HTTPCommandResponse),
					},
					Selector: recipe.Selector{
						HealthCheck: recipe.HealthCheckSelector{
							Name:  "role",
							Value: "leader",
						},
					},
				},
				{
					Name: "restart",
					Type: recipe.CommandTypeFlaps,
					FlapsCommand: recipe.FlapsCommand{
						Action: "restart",
						Method: http.MethodPost,
						Options: map[string]string{
							"force_stop": "true",
						},
					},
					Selector: recipe.Selector{
						HealthCheck: recipe.HealthCheckSelector{
							Name:  "role",
							Value: "leader",
						},
						Preprocess: true,
					},
					WaitForHealthChecks: true,
				},
			},
		}

		if err := template.Process(ctx); err != nil {
			return err
		}

	}

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
