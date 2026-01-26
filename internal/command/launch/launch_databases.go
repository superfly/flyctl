package launch

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appsecrets"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/command/mpg"
	"github.com/superfly/flyctl/internal/command/postgres"
	"github.com/superfly/flyctl/internal/command/redis"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/spinner"
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
	}

	return nil
}

func (state *launchState) createManagedPostgres(ctx context.Context) error {
	var (
		io         = iostreams.FromContext(ctx)
		pgPlan     = state.Plan.Postgres.ManagedPostgres
		uiexClient = uiexutil.ClientFromContext(ctx)
	)

	// Check if we should attach to an existing cluster instead of creating a new one
	if pgPlan.ClusterID != "" {
		return state.attachToManagedPostgres(ctx, pgPlan.ClusterID)
	}

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
		Name:         pgPlan.DbName,
		OrgSlug:      slug,
		Region:       pgPlan.Region,
		Plan:         pgPlan.Plan,
		VolumeSizeGB: pgPlan.DiskSize,
	}

	// Create cluster using the UI-EX client with retry logic for network errors
	input := uiex.CreateClusterInput{
		Name:    params.Name,
		Region:  params.Region,
		Plan:    params.Plan,
		OrgSlug: params.OrgSlug,
		Disk:    params.VolumeSizeGB,
	}

	fmt.Fprintf(io.Out, "Provisioning Managed Postgres cluster...\n")

	var response uiex.CreateClusterResponse
	err = retry.Do(
		func() error {
			var retryErr error
			response, retryErr = uiexClient.CreateCluster(ctx, input)
			return retryErr
		},
		retry.Context(ctx),
		retry.Attempts(3),
		retry.Delay(1*time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.OnRetry(func(n uint, err error) {
			fmt.Fprintf(io.Out, "Retrying cluster creation (attempt %d) due to: %v\n", n+1, err)
		}),
	)
	if err != nil {
		return fmt.Errorf("failed creating managed postgres cluster: %w", err)
	}

	// Wait for cluster to be ready
	colorize := io.ColorScheme()
	fmt.Fprintf(io.Out, "%s\n", colorize.Bold(fmt.Sprintf("Waiting for cluster %s (%s) to be ready...", params.Name, response.Data.Id)))
	fmt.Fprintf(io.Out, "\n%s\n", colorize.Bold("This'll take a few minutes, but you don't have to wait around if you don't want to!"))
	fmt.Fprintf(io.Out, "To connect your Managed Postgres cluster later just:\n")
	fmt.Fprintf(io.Out, "  - Press %s to continue with deployment\n", colorize.Purple("Ctrl+C"))
	fmt.Fprintf(io.Out, "  - Use %s to confirm that your MPG cluster is ready\n", colorize.Purple("fly mpg status"))
	fmt.Fprintf(io.Out, "  - Use %s to attach to your app\n", colorize.Purple("fly mpg attach"))

	// Start spinner to show progress
	s := spinner.Run(io, colorize.Yellow("Provisioning your Managed Postgres cluster..."))

	// Create a separate context for the wait loop with 15 minute timeout
	waitCtx := context.Background()
	waitCtx, cancel := context.WithTimeout(waitCtx, 15*time.Minute)
	defer cancel()

	// Use retry.Do with a 15-minute timeout and exponential backoff
	err = retry.Do(
		func() error {
			cluster, err := uiexClient.GetManagedClusterById(ctx, response.Data.Id)
			if err != nil {
				// For network errors, return the error to trigger retry
				if containsNetworkError(err.Error()) {
					return err
				}
				// For other errors, make them unrecoverable
				return retry.Unrecoverable(fmt.Errorf("failed checking cluster status: %w", err))
			}

			if cluster.Data.Status == "ready" {
				return nil // Success!
			}

			if cluster.Data.Status == "error" {
				return retry.Unrecoverable(fmt.Errorf("cluster creation failed"))
			}

			// Return an error to continue retrying if status is not ready
			return fmt.Errorf("cluster status is %s, waiting for ready", cluster.Data.Status)
		},
		retry.Context(waitCtx),
		retry.Attempts(0), // Unlimited attempts within the timeout
		retry.Delay(2*time.Second),
		retry.MaxDelay(30*time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.OnRetry(func(n uint, err error) {
			// Log network-related errors and periodic status updates
			if containsNetworkError(err.Error()) {
				s.Stop()
				fmt.Fprintf(io.Out, "Retrying status check due to network issue: %v\n", err)
				s = spinner.Run(io, colorize.Yellow("Provisioning your Managed Postgres cluster..."))
			} else if n%10 == 0 && n > 0 { // Log every 10th attempt to show progress
				s.Stop()
				fmt.Fprintf(io.Out, "Still waiting for cluster to be ready (attempt %d)...\n", n+1)
				s = spinner.Run(io, colorize.Yellow("Provisioning your Managed Postgres cluster..."))
			}
		}),
	)

	// Stop the spinner
	s.Stop()

	// Handle the result
	if err != nil {
		// Check if we hit the timeout
		if waitCtx.Err() == context.DeadlineExceeded {
			fmt.Fprintf(io.Out, "\nCluster creation is taking longer than expected. Continuing with deployment.\n")
			fmt.Fprintf(io.Out, "You can check the status later with 'fly mpg status' and attach with 'fly mpg attach'.\n")
			return nil
		}
		// Check if the user cancelled
		if ctx.Err() == context.Canceled {
			fmt.Fprintf(io.Out, "\nContinuing with deployment. You can check the status later with 'fly mpg status' and attach with 'fly mpg attach'.\n")
			return nil
		}
		return err
	}

	// Get the cluster credentials with retry logic
	var cluster uiex.GetManagedClusterResponse
	err = retry.Do(
		func() error {
			var retryErr error
			cluster, retryErr = uiexClient.GetManagedClusterById(ctx, response.Data.Id)
			return retryErr
		},
		retry.Context(ctx),
		retry.Attempts(3),
		retry.Delay(1*time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.OnRetry(func(n uint, err error) {
			fmt.Fprintf(io.Out, "Retrying credential retrieval (attempt %d) due to: %v\n", n+1, err)
		}),
	)
	if err != nil {
		return fmt.Errorf("failed retrieving cluster credentials: %w", err)
	}

	// Set the connection string as a secret
	secrets := map[string]string{
		"DATABASE_URL": cluster.Credentials.ConnectionUri,
	}

	flapsClient := flapsutil.ClientFromContext(ctx)
	if err := appsecrets.Update(ctx, flapsClient, state.Plan.AppName, secrets, nil); err != nil {
		return fmt.Errorf("failed setting database secrets: %w", err)
	}

	fmt.Fprintf(io.Out, "\n%s\n", colorize.Bold(colorize.Green(fmt.Sprintf("Managed Postgres cluster %s is ready and attached to %s", response.Data.Id, state.Plan.AppName))))
	fmt.Fprintf(io.Out, "The following secret was added to %s:\n  DATABASE_URL=%s\n", state.Plan.AppName, cluster.Credentials.ConnectionUri)

	return nil
}

// attachToManagedPostgres attaches an existing Managed Postgres cluster to the app
func (state *launchState) attachToManagedPostgres(ctx context.Context, clusterID string) error {
	var (
		io         = iostreams.FromContext(ctx)
		uiexClient = uiexutil.ClientFromContext(ctx)
		client     = flyutil.ClientFromContext(ctx)
	)

	// Get cluster details to verify it exists and get credentials
	fmt.Fprintf(io.Out, "Attaching to existing Managed Postgres cluster %s...\n", clusterID)

	var cluster uiex.GetManagedClusterResponse
	err := retry.Do(
		func() error {
			var retryErr error
			cluster, retryErr = uiexClient.GetManagedClusterById(ctx, clusterID)
			return retryErr
		},
		retry.Context(ctx),
		retry.Attempts(3),
		retry.Delay(1*time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.OnRetry(func(n uint, err error) {
			fmt.Fprintf(io.Out, "Retrying cluster lookup (attempt %d) due to: %v\n", n+1, err)
		}),
	)
	if err != nil {
		return fmt.Errorf("failed retrieving cluster %s: %w", clusterID, err)
	}

	// Verify the cluster and app are in the same organization
	app, err := client.GetAppBasic(ctx, state.Plan.AppName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", state.Plan.AppName, err)
	}

	clusterOrgSlug := cluster.Data.Organization.Slug
	appOrgSlug := app.Organization.RawSlug

	if appOrgSlug != clusterOrgSlug {
		return fmt.Errorf("app %s is in organization %s, but cluster %s is in organization %s. They must be in the same organization to attach",
			state.Plan.AppName, appOrgSlug, clusterID, clusterOrgSlug)
	}

	// Build connection URI with the database name from the plan (if provided)
	connectionUri := cluster.Credentials.ConnectionUri
	dbName := state.Plan.Postgres.ManagedPostgres.DbName
	if dbName != "" {
		// Parse the base connection URI and replace the database name
		parsedUri, err := url.Parse(cluster.Credentials.ConnectionUri)
		if err != nil {
			return fmt.Errorf("failed to parse connection URI: %w", err)
		}
		parsedUri.Path = "/" + dbName
		connectionUri = parsedUri.String()
	}

	// Set the connection string as a secret
	secrets := map[string]string{
		"DATABASE_URL": connectionUri,
	}

	flapsClient := flapsutil.ClientFromContext(ctx)
	if err := appsecrets.Update(ctx, flapsClient, state.Plan.AppName, secrets, nil); err != nil {
		return fmt.Errorf("failed setting database secrets: %w", err)
	}

	fmt.Fprintf(io.Out, "Managed Postgres cluster %s is now attached to %s\n", clusterID, state.Plan.AppName)
	fmt.Fprintf(io.Out, "The following secret was added to %s:\n  DATABASE_URL=%s\n", state.Plan.AppName, connectionUri)

	return nil
}

// containsNetworkError checks if an error message contains network-related error indicators
func containsNetworkError(errMsg string) bool {
	networkErrors := []string{
		"connection reset by peer",
		"connection refused",
		"timeout",
		"network is unreachable",
		"temporary failure in name resolution",
		"i/o timeout",
	}

	for _, netErr := range networkErrors {
		if contains(errMsg, netErr) {
			return true
		}
	}
	return false
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > len(substr) &&
				(stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
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
		// Filter out deprecated regions
		regions = lo.Filter(regions, func(r fly.Region, _ int) bool {
			return !r.Deprecated
		})
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
