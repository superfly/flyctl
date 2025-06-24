package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
)

func New() *cobra.Command {
	const (
		short = `Manage Managed Postgres clusters.`

		long = short + "\n"
	)

	cmd := command.New("managed-postgres", short, long, nil)

	cmd.Aliases = []string{"mpg"}

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
	)

	return cmd
}

// ClusterFromFlagOrSelect retrieves the cluster ID from the --cluster flag.
// If the flag is not set, it prompts the user to select a cluster from the available ones for the given organization.
func ClusterFromFlagOrSelect(ctx context.Context, orgSlug string) (*uiex.ManagedCluster, error) {
	clusterID := flag.GetMPGClusterID(ctx)
	uiexClient := uiexutil.ClientFromContext(ctx)

	clustersResponse, err := uiexClient.ListManagedClusters(ctx, orgSlug)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving postgres clusters: %w", err)
	}

	if len(clustersResponse.Data) == 0 {
		return nil, fmt.Errorf("no managed postgres clusters found in organization %s", orgSlug)
	}

	if clusterID != "" {
		// If a cluster ID is provided via flag, find it
		for i := range clustersResponse.Data {
			if clustersResponse.Data[i].Id == clusterID {
				return &clustersResponse.Data[i], nil
			}
		}
		return nil, fmt.Errorf("managed postgres cluster %q not found in organization %s", clusterID, orgSlug)
	} else {
		// Otherwise, prompt the user to select a cluster
		var options []string
		for _, cluster := range clustersResponse.Data {
			options = append(options, fmt.Sprintf("%s (%s)", cluster.Name, cluster.Region))
		}

		var index int
		selectErr := prompt.Select(ctx, &index, "Select a Postgres cluster", "", options...)
		if selectErr != nil {
			return nil, selectErr
		}
		return &clustersResponse.Data[index], nil
	}
}

// GetAvailableMPGRegions returns the list of regions available for Managed Postgres
func GetAvailableMPGRegions(ctx context.Context, orgSlug string) ([]fly.Region, error) {
	uiexClient := uiexutil.ClientFromContext(ctx)

	// Get platform regions
	regionsFuture := prompt.PlatformRegions(ctx)
	regions, err := regionsFuture.Get()
	if err != nil {
		return nil, err
	}

	// Try to get available MPG regions from API
	mpgRegionsResponse, err := uiexClient.ListMPGRegions(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	return filterMPGRegions(regions.Regions, mpgRegionsResponse.Data), nil
}

// IsValidMPGRegion checks if a region code is valid for Managed Postgres
func IsValidMPGRegion(ctx context.Context, orgSlug string, regionCode string) (bool, error) {
	availableRegions, err := GetAvailableMPGRegions(ctx, orgSlug)
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
	availableRegions, err := GetAvailableMPGRegions(ctx, orgSlug)
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
