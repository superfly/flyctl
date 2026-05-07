package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiex/mpg"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
)

func New() *cobra.Command {
	const (
		short = `Manage Managed Postgres clusters.`

		long = short + "\n"
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

// ClusterFromArgOrSelect retrieves the cluster if the cluster ID is passed in
// otherwise it prompts the user to select a cluster from the available ones for
// the given organization.
// It prompts for the org if the org slug is not provided.
func ClusterFromArgOrSelect(ctx context.Context, clusterID, orgSlug string) (*mpg.Cluster, string, error) {
	mpgv1Client := mpgv1.ClientFromContext(ctx)
	mpgv2Client := mpgv2.ClientFromContext(ctx)

	if clusterID != "" {
		if c, err := mpgv1Client.GetManagedClusterById(ctx, clusterID); err == nil {
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
				Version:       mpg.VersionV1,
			}

			return cluster, cluster.Organization.Slug, nil
		}
		if c, err := mpgv2Client.GetClusterById(ctx, clusterID); err == nil {
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
				Version:       mpg.VersionV2,
			}

			return cluster, cluster.Organization.Slug, nil
		}
	}

	if orgSlug == "" {
		org, err := prompt.Org(ctx)
		if err != nil {
			return nil, "", err
		}

		orgSlug = org.RawSlug
	}

	// Fetch clusters. Odd but the v1 client endpoint returns both v1 and v2 clusters,
	// they are just identified with the `Version` field being 1 or 2.
	mc, err := mpgv1Client.ListManagedClusters(ctx, orgSlug, false)
	if err != nil {
		return nil, orgSlug, fmt.Errorf("failed retrieving postgres clusters: %w", err)
	}

	if len(mc.Data) == 0 {
		return nil, orgSlug, fmt.Errorf("no managed postgres clusters found in organization %s", orgSlug)
	}

	clusters := make([]*mpg.Cluster, 0, len(mc.Data))
	for _, cluster := range mc.Data {
		version := mpg.VersionV1
		if cluster.Version == 2 {
			version = mpg.VersionV2
		}
		clusterId := ""
		if cluster.Version == 2 {
			clusterId = cluster.ClusterId
		}
		clusters = append(clusters, &mpg.Cluster{
			Id:           cluster.Id,
			ClusterId:    clusterId,
			Name:         cluster.Name,
			Region:       cluster.Region,
			Status:       cluster.Status,
			Plan:         cluster.Plan,
			Disk:         cluster.Disk,
			Replicas:     cluster.Replicas,
			Organization: cluster.Organization,
			Version:      version,
		})
	}

	// If a cluster ID is provided via flag, find it
	if clusterID != "" {
		for _, cluster := range clusters {
			if cluster.Id == clusterID {
				return cluster, orgSlug, nil
			}
		}

		return nil, orgSlug, fmt.Errorf("managed postgres cluster %q not found in organization %s", clusterID, orgSlug)
	}

	// Otherwise, prompt the user to select a cluster
	var options []string
	for _, cluster := range clusters {
		options = append(options, fmt.Sprintf("%s [%s] (%s)", cluster.Name, cluster.Id, cluster.Region))
	}

	var index int
	selectErr := prompt.Select(ctx, &index, "Select a Postgres cluster", "", options...)
	if selectErr != nil {
		return nil, orgSlug, selectErr
	}

	return clusters[index], orgSlug, nil
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
