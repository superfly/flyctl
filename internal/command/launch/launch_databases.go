package launch

import (
	"context"
	"fmt"
	"time"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/command/postgres"
	"github.com/superfly/flyctl/internal/command/redis"
	"github.com/superfly/flyctl/iostreams"
)

// createDatabases creates databases requested by the plan
func (state *launchState) createDatabases(ctx context.Context) error {

	if state.plan.Postgres.FlyPostgres != nil {
		err := state.createFlyPostgres(ctx)
		if err != nil {
			// TODO(Ali): Make error printing here better.
			fmt.Fprintf(iostreams.FromContext(ctx).ErrOut, "Error creating Postgres cluster: %s\n", err)
		}
	}

	if state.plan.Redis.UpstashRedis != nil {
		err := state.createUpstashRedis(ctx)
		if err != nil {
			// TODO(Ali): Make error printing here better.
			fmt.Fprintf(iostreams.FromContext(ctx).ErrOut, "Error creating Redis database: %s\n", err)
		}
	}

	// Run any initialization commands required for Postgres if it was installed
	if state.plan.Postgres.Provider() != nil && state.sourceInfo != nil {
		for _, cmd := range state.sourceInfo.PostgresInitCommands {
			if cmd.Condition {
				if err := execInitCommand(ctx, cmd); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (state *launchState) createFlyPostgres(ctx context.Context) error {
	var (
		pgPlan    = state.plan.Postgres.FlyPostgres
		apiClient = client.FromContext(ctx).API()
		io        = iostreams.FromContext(ctx)
	)

	attachToExisting := false

	if pgPlan.AppName == "" {
		pgPlan.AppName = fmt.Sprintf("%s-db", state.appConfig.AppName)
	}

	if apps, err := apiClient.GetApps(ctx, nil); err == nil {
		for _, app := range apps {
			if app.Name == pgPlan.AppName {
				attachToExisting = true
			}
		}
	}

	if attachToExisting {
		// If we try to attach to a PG cluster with the usual username
		// format, we'll get an error (since that username already exists)
		// by generating a new username with a sufficiently random number
		// (in this case, the nanon second that the database is being attached)
		currentTime := time.Now().Nanosecond()
		dbUser := fmt.Sprintf("%s-%d", pgPlan.AppName, currentTime)

		err := postgres.AttachCluster(ctx, postgres.AttachParams{
			PgAppName: pgPlan.AppName,
			AppName:   state.plan.AppName,
			DbUser:    dbUser,
		})

		if err != nil {
			msg := "Failed attaching %s to the Postgres cluster %s: %s.\nTry attaching manually with 'fly postgres attach --app %s %s'\n"
			fmt.Fprintf(io.Out, msg, state.plan.AppName, pgPlan.AppName, err, state.plan.AppName, pgPlan.AppName)
			return err
		} else {
			fmt.Fprintf(io.Out, "Postgres cluster %s is now attached to %s\n", pgPlan.AppName, state.plan.AppName)
		}
	} else {
		// Create new PG cluster
		org, err := state.Org(ctx)
		if err != nil {
			return err
		}
		region, err := state.Region(ctx)
		if err != nil {
			return err
		}
		err = postgres.CreateCluster(ctx, org, &region, &postgres.ClusterParams{
			PostgresConfiguration: postgres.PostgresConfiguration{
				Name:               pgPlan.AppName,
				DiskGb:             pgPlan.DiskSizeGB,
				InitialClusterSize: pgPlan.Nodes,
				VMSize:             pgPlan.VmSize,
				MemoryMb:           pgPlan.VmRam,
			},
			ScaleToZero: &pgPlan.AutoStop,
			Autostart:   true, // TODO(Ali): Do we want this?
			Manager:     flypg.ReplicationManager,
		})
		if err != nil {
			fmt.Fprintf(io.Out, "Failed creating the Postgres cluster %s: %s\n", pgPlan.AppName, err)
		} else {
			err = postgres.AttachCluster(ctx, postgres.AttachParams{
				PgAppName: pgPlan.AppName,
				AppName:   state.plan.AppName,
				SuperUser: true,
			})

			if err != nil {
				msg := "Failed attaching %s to the Postgres cluster %s: %s.\nTry attaching manually with 'fly postgres attach --app %s %s'\n"
				fmt.Fprintf(io.Out, msg, state.plan.AppName, pgPlan.AppName, err, state.plan.AppName, pgPlan.AppName)
			} else {
				fmt.Fprintf(io.Out, "Postgres cluster %s is now attached to %s\n", pgPlan.AppName, state.plan.AppName)
			}
		}

		if err != nil {
			const msg = "Error creating Postgresql database. Be warned that this may affect deploys"
			fmt.Fprintln(io.Out, io.ColorScheme().Red(msg))
		}

		return err
	}

	return nil
}

func (state *launchState) createUpstashRedis(ctx context.Context) error {
	redisPlan := state.plan.Redis.UpstashRedis
	if redisPlan.AppName == "" {
		redisPlan.AppName = fmt.Sprintf("%s-redis", state.appConfig.AppName)
	}
	org, err := state.Org(ctx)
	if err != nil {
		return err
	}
	region, err := state.Region(ctx)
	if err != nil {
		return err
	}
	// TODO: Use values from plan correctly. This ignores them and picks defaults instead.
	db, err := redis.Create(ctx, org, redisPlan.AppName, &region, redisPlan.PlanId, true, false)
	if err != nil {
		return err
	}
	return redis.AttachDatabase(ctx, db, state.plan.AppName)
}
