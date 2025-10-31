package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
)

// RegionProvider interface for getting platform regions
type RegionProvider interface {
	GetPlatformRegions(ctx context.Context) ([]fly.Region, error)
}

// DefaultRegionProvider implements RegionProvider using the prompt package
type DefaultRegionProvider struct{}

func (p *DefaultRegionProvider) GetPlatformRegions(ctx context.Context) ([]fly.Region, error) {
	regionsFuture := prompt.PlatformRegions(ctx)
	regions, err := regionsFuture.Get()
	if err != nil {
		return nil, err
	}
	return regions.Regions, nil
}

// MPGService provides MPG-related functionality with injectable dependencies
type MPGService struct {
	uiexClient     uiexutil.Client
	regionProvider RegionProvider
}

// NewMPGService creates a new MPGService with default dependencies
func NewMPGService(ctx context.Context) *MPGService {
	return &MPGService{
		uiexClient:     uiexutil.ClientFromContext(ctx),
		regionProvider: &DefaultRegionProvider{},
	}
}

// NewMPGServiceWithDependencies creates a new MPGService with custom dependencies
func NewMPGServiceWithDependencies(uiexClient uiexutil.Client, regionProvider RegionProvider) *MPGService {
	return &MPGService{
		uiexClient:     uiexClient,
		regionProvider: regionProvider,
	}
}

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
		newStatus(),
		newList(),
		newCreate(),
		newDestroy(),
		newBackup(),
		newRestore(),
	)

	return cmd
}

// ClusterFromArgOrSelect retrieves the cluster if the cluster ID is passed in
// otherwise it prompts the user to select a cluster from the available ones for
// the given organization.
// It prompts for the org if the org slug is not provided.
func ClusterFromArgOrSelect(ctx context.Context, clusterID, orgSlug string) (*uiex.ManagedCluster, string, error) {
	uiexClient := uiexutil.ClientFromContext(ctx)

	if orgSlug == "" {
		org, err := prompt.Org(ctx)
		if err != nil {
			return nil, "", err
		}

		orgSlug = org.RawSlug
	}

	clustersResponse, err := uiexClient.ListManagedClusters(ctx, orgSlug)
	if err != nil {
		return nil, orgSlug, fmt.Errorf("failed retrieving postgres clusters: %w", err)
	}

	if len(clustersResponse.Data) == 0 {
		return nil, orgSlug, fmt.Errorf("no managed postgres clusters found in organization %s", orgSlug)
	}

	if clusterID != "" {
		// If a cluster ID is provided via flag, find it
		for i := range clustersResponse.Data {
			if clustersResponse.Data[i].Id == clusterID {
				return &clustersResponse.Data[i], orgSlug, nil
			}
		}
		return nil, orgSlug, fmt.Errorf("managed postgres cluster %q not found in organization %s", clusterID, orgSlug)
	} else {
		// Otherwise, prompt the user to select a cluster
		var options []string
		for _, cluster := range clustersResponse.Data {
			options = append(options, fmt.Sprintf("%s [%s] (%s)", cluster.Name, cluster.Id, cluster.Region))
		}

		var index int
		selectErr := prompt.Select(ctx, &index, "Select a Postgres cluster", "", options...)
		if selectErr != nil {
			return nil, orgSlug, selectErr
		}
		return &clustersResponse.Data[index], orgSlug, nil
	}
}

// GetAvailableMPGRegions returns the list of regions available for Managed Postgres
func GetAvailableMPGRegions(ctx context.Context, orgSlug string) ([]fly.Region, error) {
	service := NewMPGService(ctx)
	return service.GetAvailableMPGRegions(ctx, orgSlug)
}

// GetAvailableMPGRegions returns the list of regions available for Managed Postgres
func (s *MPGService) GetAvailableMPGRegions(ctx context.Context, orgSlug string) ([]fly.Region, error) {
	// Get platform regions
	platformRegions, err := s.regionProvider.GetPlatformRegions(ctx)
	if err != nil {
		return nil, err
	}

	// Try to get available MPG regions from API
	mpgRegionsResponse, err := s.uiexClient.ListMPGRegions(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	return filterMPGRegions(platformRegions, mpgRegionsResponse.Data), nil
}

// IsValidMPGRegion checks if a region code is valid for Managed Postgres
func IsValidMPGRegion(ctx context.Context, orgSlug string, regionCode string) (bool, error) {
	service := NewMPGService(ctx)
	return service.IsValidMPGRegion(ctx, orgSlug, regionCode)
}

// IsValidMPGRegion checks if a region code is valid for Managed Postgres
func (s *MPGService) IsValidMPGRegion(ctx context.Context, orgSlug string, regionCode string) (bool, error) {
	availableRegions, err := s.GetAvailableMPGRegions(ctx, orgSlug)
	if err != nil {
		return false, err
	}

	for _, region := range availableRegions {
		if region.Code == regionCode {
			return true, nil
		}
	}
	return false, nil
}

// GetAvailableMPGRegionCodes returns just the region codes for error messages
func GetAvailableMPGRegionCodes(ctx context.Context, orgSlug string) ([]string, error) {
	service := NewMPGService(ctx)
	return service.GetAvailableMPGRegionCodes(ctx, orgSlug)
}

// GetAvailableMPGRegionCodes returns just the region codes for error messages
func (s *MPGService) GetAvailableMPGRegionCodes(ctx context.Context, orgSlug string) ([]string, error) {
	availableRegions, err := s.GetAvailableMPGRegions(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	var codes []string
	for _, region := range availableRegions {
		codes = append(codes, region.Code)
	}
	return codes, nil
}

// filterMPGRegions filters platform regions based on MPG availability
func filterMPGRegions(platformRegions []fly.Region, mpgRegions []uiex.MPGRegion) []fly.Region {
	var filteredRegions []fly.Region

	for _, region := range platformRegions {
		for _, allowed := range mpgRegions {
			if region.Code == allowed.Code && allowed.Available {
				filteredRegions = append(filteredRegions, region)
				break
			}
		}
	}

	return filteredRegions
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
