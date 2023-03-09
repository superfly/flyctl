package launch

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/command/postgres"
	"github.com/superfly/flyctl/internal/command/redis"
	"github.com/superfly/flyctl/iostreams"
)

func LaunchPostgres(ctx context.Context, appName string, org *api.Organization, region *api.Region) error {
	io := iostreams.FromContext(ctx)
	clusterAppName := appName + "-db"
	err := postgres.CreateCluster(ctx, org, region,
		&postgres.ClusterParams{
			PostgresConfiguration: postgres.PostgresConfiguration{
				Name: clusterAppName,
			},
			Manager: flypg.ReplicationManager,
		})

	if err != nil {
		fmt.Fprintf(io.Out, "Failed creating the Postgres cluster %s: %s\n", clusterAppName, err)
	} else {
		err = postgres.AttachCluster(ctx, postgres.AttachParams{
			PgAppName: clusterAppName,
			AppName:   appName,
			SuperUser: true,
		})

		if err != nil {
			msg := `Failed attaching %s to the Postgres cluster %s: %w.\nTry attaching manually with 'fly postgres attach --app %s %s'\n`
			fmt.Fprintf(io.Out, msg, appName, clusterAppName, err, appName, clusterAppName)

		} else {
			fmt.Fprintf(io.Out, "Postgres cluster %s is now attached to %s\n", clusterAppName, appName)
		}
	}

	return err
}

func LaunchRedis(ctx context.Context, appName string, org *api.Organization, region *api.Region) error {
	name := appName + "-redis"
	db, err := redis.Create(ctx, org, name, region, "", true, false)

	if err != nil {
		fmt.Println(fmt.Errorf("%w", err))
	} else {
		redis.AttachDatabase(ctx, db, appName)
	}

	return err
}
