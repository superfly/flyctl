package restart

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-version"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/command/postgres"
	"github.com/superfly/flyctl/iostreams"
)

func runNomadRestart(ctx context.Context, app *api.AppCompact) error {
	if app.PostgresAppRole != nil && app.PostgresAppRole.Name == "postgres_cluster" {
		return runNomadPGRestart(ctx, app)
	}

	return runNomadBaseRestart(ctx, app)
}

func runNomadBaseRestart(ctx context.Context, app *api.AppCompact) error {
	client := client.FromContext(ctx).API()

	if _, err := client.RestartApp(ctx, app.Name); err != nil {
		return fmt.Errorf("failed restarting app: %w", err)
	}

	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.Out, "%s is being restarted\n", app.Name)

	return nil
}

func runNomadPGRestart(ctx context.Context, app *api.AppCompact) error {
	var (
		dialer               = agent.DialerFromContext(ctx)
		client               = client.FromContext(ctx).API()
		io                   = iostreams.FromContext(ctx)
		colorize             = io.ColorScheme()
		MinPostgresHaVersion = "0.0.20"
	)

	if err := postgres.NomadPGVersionCompatible(app, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
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

func nomadNodeRoles(ctx context.Context, allocs []*api.AllocationStatus) (leader *api.AllocationStatus, replicas []*api.AllocationStatus, err error) {
	dialer := agent.DialerFromContext(ctx)

	for _, alloc := range allocs {
		pgclient := flypg.NewFromInstance(alloc.PrivateIP, dialer)
		if err != nil {
			return nil, nil, fmt.Errorf("can't connect to %s: %w", alloc.ID, err)
		}

		role, err := pgclient.NodeRole(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("can't get role for %s: %w", alloc.ID, err)
		}

		switch role {
		case "leader":
			leader = alloc
		case "replica":
			replicas = append(replicas, alloc)
		}
	}
	return leader, replicas, nil
}

func hasRequiredPGVersionOnNomad(app *api.AppCompact, cluster, standalone string) error {
	// Validate image version to ensure it's compatible with this feature.
	if app.ImageDetails.Version == "" || app.ImageDetails.Version == "unknown" {
		return fmt.Errorf("command is not compatible with this image")
	}

	imageVersionStr := app.ImageDetails.Version[1:]
	imageVersion, err := version.NewVersion(imageVersionStr)
	if err != nil {
		return err
	}

	// Specify compatible versions per repo.
	requiredVersion := &version.Version{}
	if app.ImageDetails.Repository == "flyio/postgres-standalone" {
		requiredVersion, err = version.NewVersion(standalone)
		if err != nil {
			return err
		}
	}
	if app.ImageDetails.Repository == "flyio/postgres" {
		requiredVersion, err = version.NewVersion(cluster)
		if err != nil {
			return err
		}
	}

	if requiredVersion == nil {
		return fmt.Errorf("unable to resolve image version")
	}

	if imageVersion.LessThan(requiredVersion) {
		return fmt.Errorf(
			"image version is not compatible. (Current: %s, Required: >= %s)\n"+
				"Please run 'flyctl image show' and update to the latest available version",
			imageVersion, requiredVersion.String())
	}

	return nil
}
