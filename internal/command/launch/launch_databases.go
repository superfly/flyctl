package launch

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/command/mpg"
	"github.com/superfly/flyctl/internal/command/redis"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
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

	if state.Plan.Postgres.ManagedPostgres != nil && (planStep == "" || planStep == "postgres") {
		err := state.createManagedPostgres(ctx)
		if err != nil {
			// TODO(Ali): Make error printing here better.
			fmt.Fprintf(iostreams.FromContext(ctx).ErrOut, "Error creating Managed Postgres cluster: %s\n", err)
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
		io         = iostreams.FromContext(ctx)
		pgPlan     = state.Plan.Postgres.FlyPostgres
		uiexClient = uiexutil.ClientFromContext(ctx)
	)

	// Get org and region
	org, err := state.Org(ctx)
	if err != nil {
		return err
	}
	region, err := state.Region(ctx)
	if err != nil {
		return err
	}

	var slug string
	if org.Slug == "personal" {
		genqClient := flyutil.ClientFromContext(ctx).GenqClient()

		// For ui-ex request we need the real org slug
		var fullOrg *gql.GetOrganizationResponse
		if fullOrg, err = gql.GetOrganization(ctx, genqClient, org.Slug); err != nil {
			return fmt.Errorf("failed fetching org: %w", err)
		}

		slug = fullOrg.Organization.RawSlug
	} else {
		slug = org.Slug
	}

	// Create new managed Postgres cluster
	input := uiex.CreateClusterInput{
		Name:    pgPlan.AppName,
		Region:  region.Code,
		Plan:    "basic", // Default plan for now
		OrgSlug: slug,
	}

	fmt.Fprintf(io.Out, "Provisioning Postgres cluster...\n")

	response, err := uiexClient.CreateCluster(ctx, input)
	if err != nil {
		return fmt.Errorf("failed creating managed postgres cluster: %w", err)
	}

	// Wait for cluster to be ready
	fmt.Fprintf(io.Out, "Waiting for cluster %s (%s) to be ready...\n", pgPlan.AppName, response.Data.Id)
	fmt.Fprintf(io.Out, "If this is taking too long, you can press Ctrl+C to continue with deployment.\n")
	fmt.Fprintf(io.Out, "You can check the status later with 'mpg status' and attach with 'mpg attach'.\n")

	// Create a separate context for the wait loop that won't propagate cancellation
	waitCtx := context.Background()
	waitCtx, cancel := context.WithCancel(waitCtx)
	defer cancel()

	// Channel to signal when cluster is ready
	ready := make(chan bool, 1)
	errChan := make(chan error, 1)

	// Start the wait loop in a goroutine
	go func() {
		for {
			select {
			case <-waitCtx.Done():
				return
			default:
				cluster, err := uiexClient.GetManagedClusterById(ctx, response.Data.Id)
				if err != nil {
					errChan <- fmt.Errorf("failed checking cluster status: %w", err)
					return
				}

				if cluster.Data.Status == "ready" {
					ready <- true
					return
				}

				if cluster.Data.Status == "error" {
					errChan <- fmt.Errorf("cluster creation failed")
					return
				}

				time.Sleep(5 * time.Second)
			}
		}
	}()

	// Wait for either ready signal, error, or context cancellation
	select {
	case <-ready:
		// Cluster is ready, continue with user creation
	case err := <-errChan:
		return err
	case <-ctx.Done():
		fmt.Fprintf(io.Out, "\nContinuing with deployment. You can check the status later with 'mpg status' and attach with 'mpg attach'.\n")
		// Continue with deployment even if cluster isn't ready
		return nil
	}

	// Get the cluster credentials
	cluster, err := uiexClient.GetManagedClusterById(ctx, response.Data.Id)
	if err != nil {
		return fmt.Errorf("failed retrieving cluster credentials: %w", err)
	}

	// Set the connection string as a secret
	secrets := map[string]string{
		"DATABASE_URL": cluster.Credentials.ConnectionUri,
	}

	client := flyutil.ClientFromContext(ctx)
	if _, err := client.SetSecrets(ctx, state.Plan.AppName, secrets); err != nil {
		return fmt.Errorf("failed setting database secrets: %w", err)
	}

	fmt.Fprintf(io.Out, "Postgres cluster %s is ready and attached to %s\n", response.Data.Id, state.Plan.AppName)
	fmt.Fprintf(io.Out, "The following secret was added to %s:\n  DATABASE_URL=%s\n", state.Plan.AppName, cluster.Credentials.ConnectionUri)

	return nil
}

func (state *launchState) createManagedPostgres(ctx context.Context) error {
	var (
		io         = iostreams.FromContext(ctx)
		pgPlan     = state.Plan.Postgres.ManagedPostgres
		uiexClient = uiexutil.ClientFromContext(ctx)
	)

	// Get org
	org, err := state.Org(ctx)
	if err != nil {
		return err
	}

	var slug string
	if org.Slug == "personal" {
		genqClient := flyutil.ClientFromContext(ctx).GenqClient()

		// For ui-ex request we need the real org slug
		var fullOrg *gql.GetOrganizationResponse
		if fullOrg, err = gql.GetOrganization(ctx, genqClient, org.Slug); err != nil {
			return fmt.Errorf("failed fetching org: %w", err)
		}

		slug = fullOrg.Organization.RawSlug
	} else {
		slug = org.Slug
	}

	// Create cluster using the same parameters as mpg create
	params := &mpg.CreateClusterParams{
		Name:          pgPlan.DbName,
		OrgSlug:       slug,
		Region:        pgPlan.Region,
		Plan:          pgPlan.Plan,
		Nodes:         1, // Default to single node
		VolumeSizeGB:  pgPlan.DiskSize,
		EnableBackups: true,  // Default to enabled
		AutoStop:      false, // Default to disabled
	}

	// Create cluster using the UI-EX client
	input := uiex.CreateClusterInput{
		Name:    params.Name,
		Region:  params.Region,
		Plan:    params.Plan,
		OrgSlug: params.OrgSlug,
	}

	fmt.Fprintf(io.Out, "Provisioning Managed Postgres cluster...\n")

	response, err := uiexClient.CreateCluster(ctx, input)
	if err != nil {
		return fmt.Errorf("failed creating managed postgres cluster: %w", err)
	}

	// Wait for cluster to be ready
	fmt.Fprintf(io.Out, "Waiting for cluster %s (%s) to be ready...\n", params.Name, response.Data.Id)
	fmt.Fprintf(io.Out, "If this is taking too long, you can press Ctrl+C to continue with deployment.\n")
	fmt.Fprintf(io.Out, "You can check the status later with 'mpg status' and attach with 'mpg attach'.\n")

	// Create a separate context for the wait loop that won't propagate cancellation
	waitCtx := context.Background()
	waitCtx, cancel := context.WithCancel(waitCtx)
	defer cancel()

	// Channel to signal when cluster is ready
	ready := make(chan bool, 1)
	errChan := make(chan error, 1)

	// Start the wait loop in a goroutine
	go func() {
		for {
			select {
			case <-waitCtx.Done():
				return
			default:
				cluster, err := uiexClient.GetManagedClusterById(ctx, response.Data.Id)
				if err != nil {
					errChan <- fmt.Errorf("failed checking cluster status: %w", err)
					return
				}

				if cluster.Data.Status == "ready" {
					ready <- true
					return
				}

				if cluster.Data.Status == "error" {
					errChan <- fmt.Errorf("cluster creation failed")
					return
				}

				time.Sleep(5 * time.Second)
			}
		}
	}()

	// Wait for either ready signal, error, or context cancellation
	select {
	case <-ready:
		// Cluster is ready, continue with user creation
	case err := <-errChan:
		return err
	case <-ctx.Done():
		fmt.Fprintf(io.Out, "\nContinuing with deployment. You can check the status later with 'mpg status' and attach with 'mpg attach'.\n")
		// Continue with deployment even if cluster isn't ready
		return nil
	}

	// Get the cluster credentials
	cluster, err := uiexClient.GetManagedClusterById(ctx, response.Data.Id)
	if err != nil {
		return fmt.Errorf("failed retrieving cluster credentials: %w", err)
	}

	// Set the connection string as a secret
	secrets := map[string]string{
		"DATABASE_URL": cluster.Credentials.ConnectionUri,
	}

	client := flyutil.ClientFromContext(ctx)
	if _, err := client.SetSecrets(ctx, state.Plan.AppName, secrets); err != nil {
		return fmt.Errorf("failed setting database secrets: %w", err)
	}

	fmt.Fprintf(io.Out, "Managed Postgres cluster %s is ready and attached to %s\n", response.Data.Id, state.Plan.AppName)
	fmt.Fprintf(io.Out, "The following secret was added to %s:\n  DATABASE_URL=%s\n", state.Plan.AppName, cluster.Credentials.ConnectionUri)

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
