package launch

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/gql"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/command/postgres"
	"github.com/superfly/flyctl/internal/command/redis"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/iostreams"
)

// createDatabases creates databases requested by the plan
func (state *launchState) createDatabases(ctx context.Context) error {
	planStep := plan.GetPlanStep(ctx)

	if state.Plan.Postgres.FlyPostgres != nil && (planStep == "" || planStep == "postgres") {
		err := state.createFlyPostgres(ctx)
		if err != nil {
			// TODO(Ali): Make error printing here better.
			fmt.Fprintf(iostreams.FromContext(ctx).ErrOut, "Error creating Postgres cluster: %s\n", err)
		}
	}

	if state.Plan.Postgres.SupabasePostgres != nil && (planStep == "" || planStep == "postgres") {
		fmt.Fprintf(iostreams.FromContext(ctx).ErrOut, "Supabase Postgres is no longer supported.\n")
	}

	if state.Plan.Redis.UpstashRedis != nil && (planStep == "" || planStep == "redis") {
		err := state.createUpstashRedis(ctx)
		if err != nil {
			// TODO(Ali): Make error printing here better.
			fmt.Fprintf(iostreams.FromContext(ctx).ErrOut, "Error provisioning Upstash Redis: %s\n", err)
		}
	}

	if state.Plan.ObjectStorage.TigrisObjectStorage != nil && (planStep == "" || planStep == "tigris") {
		err := state.createTigrisObjectStorage(ctx)
		if err != nil {
			// TODO(Ali): Make error printing here better.
			fmt.Fprintf(iostreams.FromContext(ctx).ErrOut, "Error creating Tigris object storage: %s\n", err)
		}
	}

	// Run any initialization commands required for Postgres if it was installed
	if state.Plan.Postgres.Provider() != nil && state.sourceInfo != nil && (planStep == "" || planStep == "postgres") {
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
		pgPlan    = state.Plan.Postgres.FlyPostgres
		apiClient = flyutil.ClientFromContext(ctx)
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
			AppName:   state.Plan.AppName,
			DbUser:    dbUser,
		})

		if err != nil {
			msg := "Failed attaching %s to the Postgres cluster %s: %s.\nTry attaching manually with 'fly postgres attach --app %s %s'\n"
			fmt.Fprintf(io.Out, msg, state.Plan.AppName, pgPlan.AppName, err, state.Plan.AppName, pgPlan.AppName)
			return err
		} else {
			fmt.Fprintf(io.Out, "Postgres cluster %s is now attached to %s\n", pgPlan.AppName, state.Plan.AppName)
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
				AppName:   state.Plan.AppName,
				SuperUser: true,
			})

			if err != nil {
				msg := "Failed attaching %s to the Postgres cluster %s: %s.\nTry attaching manually with 'fly postgres attach --app %s %s'\n"
				fmt.Fprintf(io.Out, msg, state.Plan.AppName, pgPlan.AppName, err, state.Plan.AppName, pgPlan.AppName)
			} else {
				fmt.Fprintf(io.Out, "Postgres cluster %s is now attached to %s\n", pgPlan.AppName, state.Plan.AppName)
			}
		}

		if err != nil {
			const msg = "Error creating Postgres database. Be warned that this may affect deploys"
			fmt.Fprintln(io.Out, io.ColorScheme().Red(msg))
		}

		return err
	}

	return nil
}

func (state *launchState) createUpstashRedis(ctx context.Context) error {
	redisPlan := state.Plan.Redis.UpstashRedis
	dbName := fmt.Sprintf("%s-redis", state.Plan.AppName)
	org, err := state.Org(ctx)
	if err != nil {
		return err
	}
	region, err := state.Region(ctx)
	if err != nil {
		return err
	}

	var readReplicaRegions []fly.Region
	{
		client := flyutil.ClientFromContext(ctx)
		regions, _, err := client.PlatformRegions(ctx)
		if err != nil {
			return err
		}
		for _, code := range redisPlan.ReadReplicas {
			if region, ok := lo.Find(regions, func(r fly.Region) bool { return r.Code == code }); ok {
				readReplicaRegions = append(readReplicaRegions, region)
			} else {
				return fmt.Errorf("region %s not found", code)
			}
		}
	}

	db, err := redis.Create(ctx, org, dbName, &region, len(readReplicaRegions) == 0, redisPlan.Eviction, &readReplicaRegions)
	if err != nil {
		return err
	}
	return redis.AttachDatabase(ctx, db, state.Plan.AppName)
}

func (state *launchState) createTigrisObjectStorage(ctx context.Context) error {
	tigrisPlan := state.Plan.ObjectStorage.TigrisObjectStorage

	org, err := state.Org(ctx)
	if err != nil {
		return err
	}

	params := extensions_core.ExtensionParams{
		Provider:       "tigris",
		Organization:   org,
		AppName:        state.Plan.AppName,
		OverrideName:   fly.Pointer(tigrisPlan.Name),
		OverrideRegion: state.Plan.RegionCode,
		Options: gql.AddOnOptions{
			"public":     tigrisPlan.Public,
			"accelerate": tigrisPlan.Accelerate,
			"website": map[string]interface{}{
				"domain_name": tigrisPlan.WebsiteDomainName,
			},
		},
		OverrideExtensionSecretKeyNames: state.sourceInfo.OverrideExtensionSecretKeyNames,
	}

	_, err = extensions_core.ProvisionExtension(ctx, params)

	if err != nil {
		return err
	}

	return err
}
