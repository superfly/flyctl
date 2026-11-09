package regions

import (
	"context"
	"fmt"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/prompt"

	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
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
	mpgClient      mpgv2.ClientV2
	regionProvider RegionProvider
}

// NewMPGService creates a new MPGService with default dependencies
func NewMPGService(ctx context.Context) (*MPGService, error) {
	mpgClient := mpgv2.ClientFromContext(ctx)
	if mpgClient == nil {
		return nil, fmt.Errorf("mpg client not found in context")
	}

	return &MPGService{
		mpgClient:      mpgClient,
		regionProvider: &DefaultRegionProvider{},
	}, nil
}

// NewMPGServiceWithDependencies creates a new MPGService with custom dependencies
func NewMPGServiceWithDependencies(mpgClient mpgv2.ClientV2, regionProvider RegionProvider) *MPGService {
	return &MPGService{
		mpgClient:      mpgClient,
		regionProvider: regionProvider,
	}
}

// GetAvailableMPGRegions returns the list of regions available for Managed Postgres
func GetAvailableMPGRegions(ctx context.Context, orgSlug string) ([]fly.Region, error) {
	service, err := NewMPGService(ctx)
	if err != nil {
		return nil, err
	}

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
	mpgRegionsResponse, err := s.mpgClient.ListRegions(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	return filterMPGRegions(platformRegions, mpgRegionsResponse.Data), nil
}

// IsValidMPGRegion checks if a region code is valid for Managed Postgres
func IsValidMPGRegion(ctx context.Context, orgSlug string, regionCode string) (bool, error) {
	service, err := NewMPGService(ctx)
	if err != nil {
		return false, err
	}

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
	service, err := NewMPGService(ctx)
	if err != nil {
		return nil, err
	}

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
func filterMPGRegions(platformRegions []fly.Region, mpgRegions []mpgv2.Region) []fly.Region {
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
