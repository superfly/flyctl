package mpg

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiex/mpg"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	"github.com/superfly/flyctl/iostreams"
)

const dashboardURL = "https://fly.io/dashboard"

// v2MigrationNotice is the plain-text migration notice for `fly mpg` help
// output; printV2MigrationNotice renders the styled equivalent before every
// runnable subcommand. Migration is dashboard-driven and gated by server-side
// eligibility, so both point at the dashboard rather than a CLI command and
// don't promise every cluster can migrate yet.
const v2MigrationNotice = "Managed Postgres v2 is here! Migrate eligible MPG v1 clusters to v2 from your cluster's page in the Fly.io dashboard (" + dashboardURL + ") — connection strings stay the same.\n"

func New() *cobra.Command {
	const (
		short = `Manage Managed Postgres clusters.`

		long = short + "\n\n" + v2MigrationNotice
	)

	cmd := command.New("mpg", short, long, nil)

	flag.Add(cmd,
		flag.Org(),
	)

	cmd.AddCommand(
		newProxy(),
		newConnect(),
		newAttach(),
		newDetach(),
		newStatus(),
		newList(),
		newCreate(),
		newDestroy(),
		newBackup(),
		newRestore(),
		newDatabases(),
		newUsers(),
	)

	return cmd
}

// printV2MigrationNotice announces v1 -> v2 migrations for commands that don't
// act on a single cluster; cluster-scoped commands print printV1MigrationLink
// once the resolved cluster is known to be v1.
func printV2MigrationNotice(ctx context.Context) {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	dashboard := colorize.Cyan(io.CreateLink("the Fly.io dashboard", dashboardURL))

	fmt.Fprintf(io.ErrOut, "\n%s\nMigrate eligible MPG v1 clusters to v2 from your cluster's page in %s — connection strings stay the same.\n\n",
		colorize.Yellow("Managed Postgres v2 is here!"),
		dashboard,
	)
}

// printV1MigrationLink links directly to the dashboard page that migrates the
// given cluster to MPG v2. The page is per-cluster, so this can only run after
// a command has resolved which cluster it is acting on.
func printV1MigrationLink(ctx context.Context, cluster *mpg.Cluster, orgSlug string) {
	if cluster == nil || cluster.Version != mpg.VersionV1 {
		return
	}

	if cluster.Organization.Slug != "" {
		orgSlug = cluster.Organization.Slug
	}

	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	url := fmt.Sprintf("%s/%s/managed_postgres/%s/v2-migration", dashboardURL, orgSlug, cluster.Id)

	fmt.Fprintf(io.ErrOut, "%s\n%s\n\n",
		colorize.Yellow(fmt.Sprintf("Cluster %q is on MPG v1 — migrate it to v2 at:", cluster.Name)),
		colorize.Cyan(io.CreateLinkURL(url)),
	)
}

// ClusterFromArgOrSelect retrieves the cluster if the cluster ID is passed in
// otherwise it prompts the user to select a cluster from the available ones for
// the given organization.
// It prompts for the org if the org slug is not provided.
func ClusterFromArgOrSelect(ctx context.Context, clusterID, orgSlug string) (*mpg.Cluster, string, error) {
	mpgv1Client := mpgv1.ClientFromContext(ctx)
	mpgClient := flapsutil.ClientFromContext(ctx)

	// If user told us which cluster they want
	if clusterID != "" {
		// The public Machines API is the MPGv2 source. Only a genuine not-found
		// falls back to the legacy MPGv1 client; auth and service failures must
		// remain visible instead of being misreported as a missing cluster.
		if c, err := mpgClient.GetManagedPostgresCluster(ctx, clusterID); err == nil {
			if c.ID == "" {
				return nil, orgSlug, fmt.Errorf("invalid response retrieving managed postgres cluster %q: missing cluster ID", clusterID)
			}
			cluster := clusterFromMachinesAPI(c)

			return cluster, cluster.Organization.Slug, nil
		} else if !errors.Is(err, flaps.ErrFlapsNotFound) {
			return nil, orgSlug, fmt.Errorf("failed retrieving managed postgres cluster %q: %w", clusterID, err)
		}

		if c, err := mpgv1Client.GetManagedClusterById(ctx, clusterID); err == nil {
			version := mpg.VersionV1
			if c.Data.Version == 2 {
				version = mpg.VersionV2
			}

			cluster := &mpg.Cluster{
				Id:            c.Data.Id,
				Name:          c.Data.Name,
				Region:        c.Data.Region,
				Status:        c.Data.Status,
				Plan:          c.Data.Plan,
				Disk:          c.Data.Disk,
				Replicas:      c.Data.Replicas,
				Organization:  c.Data.Organization,
				IpAssignments: c.Data.IpAssignments,
				AttachedApps:  c.Data.AttachedApps,
				Version:       version,
			}

			printV1MigrationLink(ctx, cluster, cluster.Organization.Slug)

			return cluster, cluster.Organization.Slug, nil
		}

		// There's nothing that List can tell us that Get won't, so if we didn't
		// find the cluster, let's just exit early.
		return nil, orgSlug, fmt.Errorf("managed postgres cluster %q not found", clusterID)
	}

	// Prompt for org if empty
	if orgSlug == "" {
		org, err := prompt.Org(ctx)
		if err != nil {
			return nil, "", err
		}

		orgSlug = org.RawSlug
	}

	clusters, err := listSelectableClusters(ctx, orgSlug)
	if err != nil {
		return nil, orgSlug, fmt.Errorf("failed retrieving postgres clusters: %w", err)
	}

	if len(clusters) == 0 {
		return nil, orgSlug, fmt.Errorf("no managed postgres clusters found in organization %s", orgSlug)
	}

	options := make([]string, 0, len(clusters))
	for _, cluster := range clusters {
		options = append(options, fmt.Sprintf("%s [%s] (%s)", cluster.Name, cluster.Id, cluster.Region))
	}

	var index int
	if err := prompt.Select(ctx, &index, "Select a Postgres cluster", "", options...); err != nil {
		return nil, orgSlug, err
	}

	printV1MigrationLink(ctx, clusters[index], orgSlug)

	return clusters[index], orgSlug, nil
}

func listSelectableClusters(ctx context.Context, orgSlug string) ([]*mpg.Cluster, error) {
	managedClusters, err := listManagedClusters(ctx, orgSlug, false)
	if err != nil {
		return nil, err
	}

	clusters := make([]*mpg.Cluster, 0, len(managedClusters))
	for _, cluster := range managedClusters {
		clusters = append(clusters, clusterFromLegacyAPI(cluster))
	}

	return clusters, nil
}

func listManagedClusters(ctx context.Context, orgSlug string, deleted bool) ([]mpgv1.ManagedCluster, error) {
	mpgClient := flapsutil.ClientFromContext(ctx)
	publicClusters, err := mpgClient.ListManagedPostgresClusters(ctx, flaps.ListManagedPostgresClustersRequest{
		OrgSlug:        orgSlug,
		IncludeDeleted: deleted,
	})
	publicUnavailable := errors.Is(err, flaps.ErrFlapsNotFound)
	if err != nil && !publicUnavailable {
		return nil, err
	}

	mpgv1Client := mpgv1.ClientFromContext(ctx)
	legacyClusters, err := mpgv1Client.ListManagedClusters(ctx, orgSlug, deleted)
	if err != nil {
		return nil, err
	}

	clusters := make([]mpgv1.ManagedCluster, 0, len(publicClusters)+len(legacyClusters.Data))
	publicIndexes := make(map[string]int, len(publicClusters))
	if !publicUnavailable {
		for _, cluster := range publicClusters {
			if deleted && cluster.DeletedAt == "" {
				continue
			}
			publicIndexes[cluster.ID] = len(clusters)
			clusters = append(clusters, managedClusterFromMachinesAPISummary(cluster, orgSlug))
		}
	}
	for _, cluster := range legacyClusters.Data {
		if index, duplicated := publicIndexes[cluster.Id]; duplicated {
			clusters[index].ClusterId = cluster.ClusterId
			clusters[index].Disk = cluster.Disk
			clusters[index].Replicas = cluster.Replicas
			if cluster.Organization.Slug != "" {
				clusters[index].Organization = cluster.Organization
			}
			clusters[index].IpAssignments = cluster.IpAssignments

			continue
		}
		clusters = append(clusters, cluster)
	}

	return clusters, nil
}

func clusterFromLegacyAPI(cluster mpgv1.ManagedCluster) *mpg.Cluster {
	version := mpg.VersionV1
	if cluster.Version == 2 {
		version = mpg.VersionV2
	}

	return &mpg.Cluster{
		Id:            cluster.Id,
		Name:          cluster.Name,
		Region:        cluster.Region,
		Status:        cluster.Status,
		Plan:          cluster.Plan,
		Disk:          cluster.Disk,
		Replicas:      cluster.Replicas,
		Organization:  cluster.Organization,
		IpAssignments: cluster.IpAssignments,
		AttachedApps:  cluster.AttachedApps,
		Version:       version,
	}
}

func clusterFromMachinesAPI(cluster flaps.ManagedPostgresCluster) *mpg.Cluster {
	return &mpg.Cluster{
		Id:           cluster.ID,
		Name:         cluster.Name,
		Region:       cluster.Region,
		Status:       cluster.Status,
		Plan:         cluster.Plan,
		Disk:         cluster.DiskSizeGB,
		Replicas:     cluster.Replicas,
		Organization: fly.Organization{Name: cluster.Organization.Name, Slug: cluster.Organization.Slug},
		AttachedApps: attachedAppsFromMachinesAPI(cluster.AttachedApps),
		Version:      mpg.VersionV2,
	}
}

func managedClusterFromMachinesAPISummary(cluster flaps.ManagedPostgresClusterSummary, orgSlug string) mpgv1.ManagedCluster {
	return mpgv1.ManagedCluster{
		Id:           cluster.ID,
		Name:         cluster.Name,
		Region:       cluster.Region,
		Status:       cluster.Status,
		Plan:         cluster.Plan,
		Organization: fly.Organization{Slug: orgSlug},
		AttachedApps: attachedAppsFromMachinesAPI(cluster.AttachedApps),
		Version:      2,
	}
}

func attachedAppsFromMachinesAPI(apps []flaps.ManagedPostgresAttachedApp) []mpg.AttachedApp {
	result := make([]mpg.AttachedApp, 0, len(apps))
	for _, app := range apps {
		result = append(result, mpg.AttachedApp{Name: app.Name})
	}

	return result
}

func organizationSlugMatches(org *fly.OrganizationBasic, slug string) bool {
	return org != nil && (slug == org.RawSlug || slug == org.Slug)
}

// ClusterFromFlagOrSelect retrieves the cluster ID from the --cluster flag.
// If the flag is not set, it prompts the user to select a cluster from the available ones for the given organization.
func ClusterFromFlagOrSelect(ctx context.Context, orgSlug string) (*mpg.Cluster, error) {
	clusterID := flag.GetMPGClusterID(ctx)
	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, orgSlug)

	return cluster, err
}

// AliasedOrganizationSlug resolves organization slug the aliased slug
// using GraphQL.
//
// Example:
//
//	Input:  "jon-phenow"
//	Output: "personal" (if "jon-phenow" is an alias for "personal")
//
// GraphQL Query:
//
//	query {
//	    organization(slug: "jon-phenow"){
//	        slug
//	    }
//	}
//
// Response:
//
//	{
//	    "data": {
//	        "organization": {
//	            "slug": "personal"
//	        }
//	    }
//	}
func AliasedOrganizationSlug(ctx context.Context, inputSlug string) (string, error) {
	client := flyutil.ClientFromContext(ctx)
	genqClient := client.GenqClient()

	// Query the GraphQL API to resolve the organization slug
	resp, err := gql.GetOrganization(ctx, genqClient, inputSlug)
	if err != nil {
		return "", fmt.Errorf("failed to resolve organization slug %q: %w", inputSlug, err)
	}

	// Return the canonical slug from the API response
	return resp.Organization.Slug, nil
}

// ResolveOrganizationSlug resolves organization slug aliases to the canonical slug
// using GraphQL. This handles cases where users use aliases that map to different
// canonical organization slugs.
//
// Example:
//
//	Input:  "personal"
//	Output: "jon-phenow" (if "personal" is an alias for "jon-phenow")
//
// GraphQL Query:
//
//	query {
//	    organization(slug: "personal"){
//	        rawSlug
//	    }
//	}
//
// Response:
//
//	{
//	    "data": {
//	        "organization": {
//	            "rawSlug": "jon-phenow"
//	        }
//	    }
//	}
func ResolveOrganizationSlug(ctx context.Context, inputSlug string) (string, error) {
	client := flyutil.ClientFromContext(ctx)
	genqClient := client.GenqClient()

	// Query the GraphQL API to resolve the organization slug
	resp, err := gql.GetOrganization(ctx, genqClient, inputSlug)
	if err != nil {
		return "", fmt.Errorf("failed to resolve organization slug %q: %w", inputSlug, err)
	}

	// Return the canonical slug from the API response
	return resp.Organization.RawSlug, nil
}

// requireMacaroonToken is a preparer that validates token compatibility for MPG commands.
func requireMacaroonToken(ctx context.Context) (context.Context, error) {
	if err := validateMPGTokenCompatibility(ctx); err != nil {
		return ctx, err
	}

	return ctx, nil
}

// detectTokenHasMacaroon determines if the current context has macaroon-style tokens.
// MPG commands require macaroon tokens to function properly.
func detectTokenHasMacaroon(ctx context.Context) bool {
	tokens := config.Tokens(ctx)
	if tokens == nil {
		return false
	}

	// Check for macaroon tokens (newer style)
	return len(tokens.GetMacaroonTokens()) > 0
}

// validateMPGTokenCompatibility checks if the current authentication tokens are compatible
// with MPG commands. MPG requires macaroon-style tokens and cannot work with older bearer tokens.
// Returns an error if bearer tokens are detected, suggesting the user upgrade their tokens.
func validateMPGTokenCompatibility(ctx context.Context) error {
	if !detectTokenHasMacaroon(ctx) {
		return fmt.Errorf(`MPG commands require updated tokens but found older-style tokens.

Please upgrade your authentication by running:
  flyctl auth logout
  flyctl auth login
`)
	}

	return nil
}
