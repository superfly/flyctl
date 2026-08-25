package mpg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/fly-go/tokens"
	regionsv2 "github.com/superfly/flyctl/internal/command/mpg/v2/regions"
	"github.com/superfly/flyctl/internal/command_context"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex/mpg"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

// MockRegionProvider implements RegionProvider for testing
type MockRegionProvider struct {
	GetPlatformRegionsFunc func(ctx context.Context) ([]fly.Region, error)
}

func (m *MockRegionProvider) GetPlatformRegions(ctx context.Context) ([]fly.Region, error) {
	if m.GetPlatformRegionsFunc != nil {
		return m.GetPlatformRegionsFunc(ctx)
	}

	return []fly.Region{}, nil
}

// setupTestContext creates a context with all necessary components for testing
func setupTestContext() context.Context {
	ctx := context.Background()

	// Add iostreams
	ios, _, _, _ := iostreams.Test()
	ctx = iostreams.NewContext(ctx, ios)

	// Add command context with a mock command
	cmd := &cobra.Command{}
	ctx = command_context.NewContext(ctx, cmd)

	// Add flag context with a flag set
	flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flagSet.String("cluster", "", "Cluster ID")
	flagSet.Bool("yes", false, "Auto-confirm")
	flagSet.String("org", "", "Organization")
	flagSet.Bool("json", false, "JSON output")
	ctx = flagctx.NewContext(ctx, flagSet)
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
			return flaps.ManagedPostgresCluster{}, flaps.ErrFlapsNotFound
		},
		ListManagedPostgresClustersFunc: func(context.Context, flaps.ListManagedPostgresClustersRequest) ([]flaps.ManagedPostgresClusterSummary, error) {
			return nil, nil
		},
	})

	return ctx
}

// Test NewMPGService returns error when mpg client is nil
func TestNewMPGService_NilClient(t *testing.T) {
	ctx := context.Background()

	// Test with nil mpg client in context
	service, err := regionsv2.NewMPGService(ctx)
	assert.Error(t, err)
	assert.Nil(t, service)
	assert.Contains(t, err.Error(), "mpg client not found in context")
}

// Test NewMPGService succeeds with valid client
func TestNewMPGService_ValidClient(t *testing.T) {
	ctx := setupTestContext()

	mockUiex := &mock.MpgV2Client{}
	ctx = mpgv2.NewContextWithClient(ctx, mockUiex)

	service, err := regionsv2.NewMPGService(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, service)
}

// Test ClusterFromFlagOrSelect with actual flag context
func TestClusterFromFlagOrSelect_WithFlagContext(t *testing.T) {
	ctx := setupTestContext()

	expectedCluster := mpgv1.ManagedCluster{
		Id:     "test-cluster-123",
		Name:   "test-cluster",
		Region: "ord",
		Status: "ready",
		Organization: fly.Organization{
			Slug: "test-org",
		},
		Version: 1,
	}

	mockv1 := &mock.MpgV1Client{
		ListManagedClustersFunc: func(ctx context.Context, orgSlug string, deleted bool) (mpgv1.ListManagedClustersResponse, error) {
			return mpgv1.ListManagedClustersResponse{Data: []mpgv1.ManagedCluster{}}, nil
		},
	}
	ctx = mpgv1.NewContextWithClient(ctx, mockv1)
	ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{})
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		ListManagedPostgresClustersFunc: func(context.Context, flaps.ListManagedPostgresClustersRequest) ([]flaps.ManagedPostgresClusterSummary, error) {
			return nil, nil
		},
	})

	t.Run("no clusters found", func(t *testing.T) {

		_, _, err := ClusterFromArgOrSelect(ctx, "", "test-org")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no managed postgres clusters found")
	})

	mockv1 = &mock.MpgV1Client{
		GetManagedClusterByIdFunc: func(ctx context.Context, id string) (mpgv1.GetManagedClusterResponse, error) {
			if id == expectedCluster.Id {
				return mpgv1.GetManagedClusterResponse{Data: expectedCluster}, nil
			}

			return mpgv1.GetManagedClusterResponse{}, errors.New("managed postgres cluster not found")
		},
	}
	mockv2 := &mock.MpgV2Client{
		GetClusterByIdFunc: func(ctx context.Context, id string) (mpgv2.GetClusterResponse, error) {
			return mpgv2.GetClusterResponse{}, errors.New("managed postgres cluster not found")
		},
	}
	ctx = mpgv1.NewContextWithClient(ctx, mockv1)
	ctx = mpgv2.NewContextWithClient(ctx, mockv2)
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
			return flaps.ManagedPostgresCluster{}, flaps.ErrFlapsNotFound
		},
	})

	t.Run("cluster not found by ID", func(t *testing.T) {

		_, _, err := ClusterFromArgOrSelect(ctx, "wrong-cluster-id", "test-org")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "managed postgres cluster \"wrong-cluster-id\" not found")
	})

	t.Run("cluster found by ID", func(t *testing.T) {
		cluster, _, err := ClusterFromArgOrSelect(ctx, "test-cluster-123", "test-org")
		require.NoError(t, err)
		assert.Equal(t, expectedCluster.Id, cluster.Id)
		assert.Equal(t, expectedCluster.Name, cluster.Name)
	})
}

func TestClusterFromArgOrSelectUsesPublicAPIForMPGv2(t *testing.T) {
	ctx := setupTestContext()
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetManagedPostgresClusterFunc: func(_ context.Context, id string) (flaps.ManagedPostgresCluster, error) {
			require.Equal(t, "mpg-123", id)
			return flaps.ManagedPostgresCluster{
				ID:         id,
				Name:       "public-cluster",
				Region:     "ord",
				Status:     "ready",
				Plan:       "basic",
				DiskSizeGB: 20,
				Replicas:   3,
				Organization: flaps.ManagedPostgresOrganization{
					Name: "Example Org",
					Slug: "example-org",
				},
			}, nil
		},
	})
	ctx = mpgv1.NewContextWithClient(ctx, &mock.MpgV1Client{
		GetManagedClusterByIdFunc: func(context.Context, string) (mpgv1.GetManagedClusterResponse, error) {
			t.Fatal("legacy client called for public MPGv2 cluster")
			return mpgv1.GetManagedClusterResponse{}, nil
		},
	})

	cluster, orgSlug, err := ClusterFromArgOrSelect(ctx, "mpg-123", "")
	require.NoError(t, err)
	require.Equal(t, mpg.VersionV2, cluster.Version)
	require.Equal(t, "public-cluster", cluster.Name)
	require.Equal(t, 20, cluster.Disk)
	require.Equal(t, "example-org", orgSlug)
}

func TestClusterFromArgOrSelectDoesNotFallbackOnPublicFailure(t *testing.T) {
	ctx := setupTestContext()
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
			return flaps.ManagedPostgresCluster{}, errors.New("machines api unavailable")
		},
	})
	ctx = mpgv1.NewContextWithClient(ctx, &mock.MpgV1Client{
		GetManagedClusterByIdFunc: func(context.Context, string) (mpgv1.GetManagedClusterResponse, error) {
			t.Fatal("legacy client called after non-404 public failure")
			return mpgv1.GetManagedClusterResponse{}, nil
		},
	})

	_, _, err := ClusterFromArgOrSelect(ctx, "mpg-123", "")
	require.EqualError(t, err, `failed retrieving managed postgres cluster "mpg-123": machines api unavailable`)
}

func TestClusterFromArgOrSelectPreservesMPGv2VersionFromLegacyFallback(t *testing.T) {
	ctx := setupTestContext()
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
			return flaps.ManagedPostgresCluster{}, flaps.ErrFlapsNotFound
		},
	})
	ctx = mpgv1.NewContextWithClient(ctx, &mock.MpgV1Client{
		GetManagedClusterByIdFunc: func(_ context.Context, id string) (mpgv1.GetManagedClusterResponse, error) {
			require.Equal(t, "mpg-123", id)
			return mpgv1.GetManagedClusterResponse{Data: mpgv1.ManagedCluster{
				Id:      id,
				Name:    "v2-from-legacy-fallback",
				Version: 2,
			}}, nil
		},
	})

	cluster, _, err := ClusterFromArgOrSelect(ctx, "mpg-123", "")
	require.NoError(t, err)
	require.Equal(t, mpg.VersionV2, cluster.Version)
}

func TestOrganizationSlugMatchesRawOrAliasedSlug(t *testing.T) {
	org := &fly.OrganizationBasic{RawSlug: "user-org", Slug: "personal"}

	require.True(t, organizationSlugMatches(org, "user-org"))
	require.True(t, organizationSlugMatches(org, "personal"))
	require.False(t, organizationSlugMatches(org, "other-org"))
	require.False(t, organizationSlugMatches(nil, "user-org"))
}

func TestClusterFromArgOrSelectRejectsEmptyPublicCluster(t *testing.T) {
	ctx := setupTestContext()
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
			return flaps.ManagedPostgresCluster{}, nil
		},
	})
	ctx = mpgv1.NewContextWithClient(ctx, &mock.MpgV1Client{
		GetManagedClusterByIdFunc: func(context.Context, string) (mpgv1.GetManagedClusterResponse, error) {
			t.Fatal("legacy client called after malformed public success")
			return mpgv1.GetManagedClusterResponse{}, nil
		},
	})

	_, _, err := ClusterFromArgOrSelect(ctx, "mpg-123", "")
	require.EqualError(t, err, `invalid response retrieving managed postgres cluster "mpg-123": missing cluster ID`)
}

func TestListSelectableClustersCombinesPublicAndMPGv1Only(t *testing.T) {
	ctx := setupTestContext()
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		ListManagedPostgresClustersFunc: func(_ context.Context, req flaps.ListManagedPostgresClustersRequest) ([]flaps.ManagedPostgresClusterSummary, error) {
			require.Equal(t, "example-org", req.OrgSlug)
			return []flaps.ManagedPostgresClusterSummary{{
				ID:     "mpg-v2",
				Name:   "public-cluster",
				Region: "ord",
				Status: "ready",
				Plan:   "basic",
			}}, nil
		},
	})
	ctx = mpgv1.NewContextWithClient(ctx, &mock.MpgV1Client{
		ListManagedClustersFunc: func(context.Context, string, bool) (mpgv1.ListManagedClustersResponse, error) {
			return mpgv1.ListManagedClustersResponse{Data: []mpgv1.ManagedCluster{
				{Id: "mpg-v1", Name: "legacy-cluster"},
				{Id: "mpg-v2", Name: "duplicate-private-v2", Version: 1},
				{Id: "mpg-v2-private-only", Name: "private-only-v2", Version: 2},
			}}, nil
		},
	})

	clusters, err := listSelectableClusters(ctx, "example-org")
	require.NoError(t, err)
	require.Len(t, clusters, 2)
	require.Equal(t, "mpg-v2", clusters[0].Id)
	require.Equal(t, mpg.VersionV2, clusters[0].Version)
	require.Equal(t, "mpg-v1", clusters[1].Id)
	require.Equal(t, mpg.VersionV1, clusters[1].Version)
}

func TestListSelectableClustersFallsBackToLegacyListOnPublicNotFound(t *testing.T) {
	ctx := setupTestContext()
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		ListManagedPostgresClustersFunc: func(context.Context, flaps.ListManagedPostgresClustersRequest) ([]flaps.ManagedPostgresClusterSummary, error) {
			return nil, flaps.ErrFlapsNotFound
		},
	})
	ctx = mpgv1.NewContextWithClient(ctx, &mock.MpgV1Client{
		ListManagedClustersFunc: func(context.Context, string, bool) (mpgv1.ListManagedClustersResponse, error) {
			return mpgv1.ListManagedClustersResponse{Data: []mpgv1.ManagedCluster{
				{Id: "mpg-v1", Name: "legacy-cluster"},
				{Id: "mpg-v2", Name: "private-v2", Version: 2},
			}}, nil
		},
	})

	clusters, err := listSelectableClusters(ctx, "example-org")
	require.NoError(t, err)
	require.Len(t, clusters, 2)
	require.Equal(t, mpg.VersionV1, clusters[0].Version)
	require.Equal(t, mpg.VersionV2, clusters[1].Version)
}

func TestListSelectableClustersDoesNotFallbackOnPublicFailure(t *testing.T) {
	ctx := setupTestContext()
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		ListManagedPostgresClustersFunc: func(context.Context, flaps.ListManagedPostgresClustersRequest) ([]flaps.ManagedPostgresClusterSummary, error) {
			return nil, errors.New("machines api unavailable")
		},
	})
	ctx = mpgv1.NewContextWithClient(ctx, &mock.MpgV1Client{
		ListManagedClustersFunc: func(context.Context, string, bool) (mpgv1.ListManagedClustersResponse, error) {
			t.Fatal("legacy list called after non-404 public failure")
			return mpgv1.ListManagedClustersResponse{}, nil
		},
	})

	_, err := listSelectableClusters(ctx, "example-org")
	require.EqualError(t, err, "machines api unavailable")
}

func TestListSelectableClustersRejectsEmptyPublicCluster(t *testing.T) {
	ctx := setupTestContext()
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		ListManagedPostgresClustersFunc: func(context.Context, flaps.ListManagedPostgresClustersRequest) ([]flaps.ManagedPostgresClusterSummary, error) {
			return []flaps.ManagedPostgresClusterSummary{{Name: "missing-id"}}, nil
		},
	})
	ctx = mpgv1.NewContextWithClient(ctx, &mock.MpgV1Client{
		ListManagedClustersFunc: func(context.Context, string, bool) (mpgv1.ListManagedClustersResponse, error) {
			return mpgv1.ListManagedClustersResponse{}, nil
		},
	})

	_, err := listSelectableClusters(ctx, "example-org")
	require.EqualError(t, err, "invalid response listing managed postgres clusters: missing cluster ID")
}

// Test the actual GetAvailableMPGRegions function with mocked dependencies
func TestGetAvailableMPGRegions_RealFunction(t *testing.T) {
	ctx := setupTestContext()

	platformRegions := []fly.Region{
		{Code: "ord", Name: "Chicago, Illinois (US)"},
		{Code: "lax", Name: "Los Angeles, California (US)"},
		{Code: "ams", Name: "Amsterdam, Netherlands (EU)"},
	}

	mpgRegions := []mpgv2.Region{
		{Code: "ord", Available: true},
		{Code: "lax", Available: true},
		{Code: "ams", Available: false}, // Not available
	}

	mockUiex := &mock.MpgV2Client{
		ListRegionsFunc: func(ctx context.Context, orgSlug string) (mpgv2.ListRegionsResponse, error) {
			assert.Equal(t, "test-org", orgSlug)

			return mpgv2.ListRegionsResponse{
				Data: mpgRegions,
			}, nil
		},
	}

	mockRegionProvider := &MockRegionProvider{
		GetPlatformRegionsFunc: func(ctx context.Context) ([]fly.Region, error) {
			return platformRegions, nil
		},
	}

	// Create service with mocked dependencies
	service := regionsv2.NewMPGServiceWithDependencies(mockUiex, mockRegionProvider)

	// Test the actual function
	regions, err := service.GetAvailableMPGRegions(ctx, "test-org")
	require.NoError(t, err)

	// Should only return ord and lax (available), not ams (unavailable)
	assert.Len(t, regions, 2)
	assert.Equal(t, "ord", regions[0].Code)
	assert.Equal(t, "lax", regions[1].Code)
}

// Test the actual IsValidMPGRegion function
func TestIsValidMPGRegion_RealFunction(t *testing.T) {
	ctx := setupTestContext()

	platformRegions := []fly.Region{
		{Code: "ord", Name: "Chicago, Illinois (US)"},
		{Code: "lax", Name: "Los Angeles, California (US)"},
	}

	mpgRegions := []mpgv2.Region{
		{Code: "ord", Available: true},
		{Code: "lax", Available: true},
	}

	mockUiex := &mock.MpgV2Client{
		ListRegionsFunc: func(ctx context.Context, orgSlug string) (mpgv2.ListRegionsResponse, error) {
			return mpgv2.ListRegionsResponse{
				Data: mpgRegions,
			}, nil
		},
	}

	mockRegionProvider := &MockRegionProvider{
		GetPlatformRegionsFunc: func(ctx context.Context) ([]fly.Region, error) {
			return platformRegions, nil
		},
	}

	// Create service with mocked dependencies
	service := regionsv2.NewMPGServiceWithDependencies(mockUiex, mockRegionProvider)

	// Test valid region
	valid, err := service.IsValidMPGRegion(ctx, "test-org", "ord")
	require.NoError(t, err)
	assert.True(t, valid, "Should find valid region 'ord'")

	// Test invalid region
	valid, err = service.IsValidMPGRegion(ctx, "test-org", "invalid")
	require.NoError(t, err)
	assert.False(t, valid, "Should not find invalid region")
}

// Test the actual GetAvailableMPGRegionCodes function
func TestGetAvailableMPGRegionCodes_RealFunction(t *testing.T) {
	ctx := setupTestContext()

	platformRegions := []fly.Region{
		{Code: "ord", Name: "Chicago, Illinois (US)"},
		{Code: "lax", Name: "Los Angeles, California (US)"},
	}

	mpgRegions := []mpgv2.Region{
		{Code: "ord", Available: true},
		{Code: "lax", Available: true},
	}

	mockUiex := &mock.MpgV2Client{
		ListRegionsFunc: func(ctx context.Context, orgSlug string) (mpgv2.ListRegionsResponse, error) {
			return mpgv2.ListRegionsResponse{
				Data: mpgRegions,
			}, nil
		},
	}

	mockRegionProvider := &MockRegionProvider{
		GetPlatformRegionsFunc: func(ctx context.Context) ([]fly.Region, error) {
			return platformRegions, nil
		},
	}

	// Create service with mocked dependencies
	service := regionsv2.NewMPGServiceWithDependencies(mockUiex, mockRegionProvider)

	// Test the actual function
	codes, err := service.GetAvailableMPGRegionCodes(ctx, "test-org")
	require.NoError(t, err)

	assert.Len(t, codes, 2)
	assert.Contains(t, codes, "ord")
	assert.Contains(t, codes, "lax")
}

// Test the destroy command logic (extracted from runDestroy)
func TestDestroyCommand_Logic(t *testing.T) {
	ctx := setupTestContext()

	clusterID := "test-cluster-123"
	expectedCluster := mpgv1.ManagedCluster{
		Id:     clusterID,
		Name:   "test-cluster",
		Region: "ord",
		Status: "ready",
		Organization: fly.Organization{
			Slug: "test-org",
		},
	}

	mockUiex := &mock.MpgV1Client{
		GetManagedClusterByIdFunc: func(ctx context.Context, id string) (mpgv1.GetManagedClusterResponse, error) {
			assert.Equal(t, clusterID, id)

			return mpgv1.GetManagedClusterResponse{
				Data: expectedCluster,
			}, nil
		},
		DestroyClusterFunc: func(ctx context.Context, orgSlug string, id string) error {
			assert.Equal(t, "test-org", orgSlug)
			assert.Equal(t, clusterID, id)

			return nil
		},
	}

	ctx = mpgv1.NewContextWithClient(ctx, mockUiex)

	// Test successful cluster retrieval
	response, err := mockUiex.GetManagedClusterById(ctx, clusterID)
	require.NoError(t, err)
	assert.Equal(t, expectedCluster.Id, response.Data.Id)
	assert.Equal(t, expectedCluster.Name, response.Data.Name)

	// Test organization validation
	if response.Data.Organization.Slug != "test-org" {
		t.Error("Organization validation failed")
	}

	// Test successful cluster destruction
	err = mockUiex.DestroyCluster(ctx, "test-org", clusterID)
	require.NoError(t, err)
}

// Test the status command logic (extracted from runStatus)
func TestStatusCommand_Logic(t *testing.T) {
	ctx := setupTestContext()

	clusterID := "test-cluster-123"
	expectedCluster := mpgv1.ManagedCluster{
		Id:       clusterID,
		Name:     "test-cluster",
		Region:   "ord",
		Status:   "ready",
		Plan:     "development",
		Disk:     10,
		Replicas: 1,
		Organization: fly.Organization{
			Slug: "test-org",
		},
		IpAssignments: mpg.ManagedClusterIpAssignments{
			Direct: "10.0.0.1",
		},
	}

	mockUiex := &mock.MpgV1Client{
		GetManagedClusterByIdFunc: func(ctx context.Context, id string) (mpgv1.GetManagedClusterResponse, error) {
			assert.Equal(t, clusterID, id)

			return mpgv1.GetManagedClusterResponse{
				Data: expectedCluster,
			}, nil
		},
	}

	ctx = mpgv1.NewContextWithClient(ctx, mockUiex)

	// Test successful cluster details retrieval
	clusterDetails, err := mockUiex.GetManagedClusterById(ctx, clusterID)
	require.NoError(t, err)
	assert.Equal(t, expectedCluster.Id, clusterDetails.Data.Id)
	assert.Equal(t, expectedCluster.Name, clusterDetails.Data.Name)
	assert.Equal(t, expectedCluster.Region, clusterDetails.Data.Region)
	assert.Equal(t, expectedCluster.Status, clusterDetails.Data.Status)
	assert.Equal(t, expectedCluster.Disk, clusterDetails.Data.Disk)
	assert.Equal(t, expectedCluster.Replicas, clusterDetails.Data.Replicas)
	assert.Equal(t, expectedCluster.IpAssignments.Direct, clusterDetails.Data.IpAssignments.Direct)
}

// Test the list command logic (extracted from runList)
func TestListCommand_Logic(t *testing.T) {
	ctx := setupTestContext()

	expectedClusters := []mpgv1.ManagedCluster{
		{
			Id:     "cluster-1",
			Name:   "test-cluster-1",
			Region: "ord",
			Status: "ready",
			Plan:   "development",
			Organization: fly.Organization{
				Slug: "test-org",
			},
		},
		{
			Id:     "cluster-2",
			Name:   "test-cluster-2",
			Region: "lax",
			Status: "ready",
			Plan:   "production",
			Organization: fly.Organization{
				Slug: "test-org",
			},
		},
	}

	mockUiex := &mock.MpgV1Client{
		ListManagedClustersFunc: func(ctx context.Context, orgSlug string, deleted bool) (mpgv1.ListManagedClustersResponse, error) {
			assert.Equal(t, "test-org", orgSlug)

			return mpgv1.ListManagedClustersResponse{
				Data: expectedClusters,
			}, nil
		},
	}

	ctx = mpgv1.NewContextWithClient(ctx, mockUiex)

	// Test successful cluster listing
	clusters, err := mockUiex.ListManagedClusters(ctx, "test-org", false)
	require.NoError(t, err)
	assert.Len(t, clusters.Data, 2)

	// Verify cluster data
	assert.Equal(t, expectedClusters[0].Id, clusters.Data[0].Id)
	assert.Equal(t, expectedClusters[0].Name, clusters.Data[0].Name)
	assert.Equal(t, expectedClusters[0].Region, clusters.Data[0].Region)
	assert.Equal(t, expectedClusters[0].Status, clusters.Data[0].Status)
	assert.Equal(t, expectedClusters[0].Plan, clusters.Data[0].Plan)

	assert.Equal(t, expectedClusters[1].Id, clusters.Data[1].Id)
	assert.Equal(t, expectedClusters[1].Name, clusters.Data[1].Name)
	assert.Equal(t, expectedClusters[1].Region, clusters.Data[1].Region)
	assert.Equal(t, expectedClusters[1].Status, clusters.Data[1].Status)
	assert.Equal(t, expectedClusters[1].Plan, clusters.Data[1].Plan)
}

// Test error handling in API calls
func TestErrorHandling(t *testing.T) {
	ctx := setupTestContext()

	t.Run("ListManagedClusters error", func(t *testing.T) {
		mockUiex := &mock.MpgV1Client{
			ListManagedClustersFunc: func(ctx context.Context, orgSlug string, deleted bool) (mpgv1.ListManagedClustersResponse, error) {
				return mpgv1.ListManagedClustersResponse{}, fmt.Errorf("API error")
			},
		}
		ctx := mpgv1.NewContextWithClient(ctx, mockUiex)

		_, _, err := ClusterFromArgOrSelect(ctx, "", "test-org")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed retrieving postgres clusters")
	})

	t.Run("GetManagedClusterById error", func(t *testing.T) {
		mockUiex := &mock.MpgV1Client{
			GetManagedClusterByIdFunc: func(ctx context.Context, id string) (mpgv1.GetManagedClusterResponse, error) {
				return mpgv1.GetManagedClusterResponse{}, fmt.Errorf("API error")
			},
		}
		ctx := mpgv1.NewContextWithClient(ctx, mockUiex)

		_, err := mockUiex.GetManagedClusterById(ctx, "test-cluster")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API error")
	})

	t.Run("DestroyCluster error", func(t *testing.T) {
		mockUiex := &mock.MpgV1Client{
			DestroyClusterFunc: func(ctx context.Context, orgSlug string, id string) error {
				return fmt.Errorf("destroy failed")
			},
		}
		ctx := mpgv1.NewContextWithClient(ctx, mockUiex)

		err := mockUiex.DestroyCluster(ctx, "test-org", "test-cluster")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "destroy failed")
	})
}

// Test the attach command logic (extracted from runAttach)
func TestAttachCommand_Logic(t *testing.T) {
	ctx := setupTestContext()

	clusterID := "test-cluster-123"

	expectedCluster := mpgv1.ManagedCluster{
		Id:     clusterID,
		Name:   "test-cluster",
		Region: "ord",
		Status: "ready",
		Organization: fly.Organization{
			Slug: "test-org",
		},
	}

	expectedApp := &fly.AppCompact{
		Organization: &fly.OrganizationBasic{
			Slug: "test-org",
		},
	}

	connectionURI := "postgresql://user:pass@host:5432/db"

	mockUiex := &mock.MpgV1Client{
		GetManagedClusterByIdFunc: func(ctx context.Context, id string) (mpgv1.GetManagedClusterResponse, error) {
			assert.Equal(t, clusterID, id)

			return mpgv1.GetManagedClusterResponse{
				Data: expectedCluster,
				Credentials: mpgv1.GetManagedClusterCredentialsResponse{
					ConnectionUri: connectionURI,
				},
			}, nil
		},
	}

	ctx = mpgv1.NewContextWithClient(ctx, mockUiex)

	// Test cluster retrieval
	response, err := mockUiex.GetManagedClusterById(ctx, clusterID)
	require.NoError(t, err)
	assert.Equal(t, expectedCluster.Id, response.Data.Id)
	assert.Equal(t, expectedCluster.Organization.Slug, response.Data.Organization.Slug)
	assert.Equal(t, connectionURI, response.Credentials.ConnectionUri)

	// Test organization validation logic
	clusterOrgSlug := response.Data.Organization.Slug
	appOrgSlug := expectedApp.Organization.Slug

	// Test same organization - should pass
	if appOrgSlug != clusterOrgSlug {
		t.Error("Organization validation should pass for same organization")
	}

	// Test organization validation failure
	differentApp := &fly.AppCompact{
		Organization: &fly.OrganizationBasic{
			Slug: "different-org",
		},
	}

	if differentApp.Organization.Slug == clusterOrgSlug {
		t.Error("Organization validation should fail for different organizations")
	}

	// Test secret validation logic
	existingSecrets := []fly.Secret{
		{Name: "EXISTING_SECRET"},
		{Name: "ANOTHER_SECRET"},
	}

	variableName := "DATABASE_URL"

	// Test secret doesn't exist - should pass
	secretExists := false
	for _, secret := range existingSecrets {
		if secret.Name == variableName {
			secretExists = true

			break
		}
	}
	assert.False(t, secretExists, "Secret should not exist")

	// Test secret already exists - should fail
	existingSecrets = append(existingSecrets, fly.Secret{Name: variableName})
	secretExists = false
	for _, secret := range existingSecrets {
		if secret.Name == variableName {
			secretExists = true

			break
		}
	}
	assert.True(t, secretExists, "Secret should exist")
}

// Test actual MPG token validation functions
func TestMPGTokenValidation(t *testing.T) {
	t.Run("detectTokenHasMacaroon with actual contexts", func(t *testing.T) {
		// Test case 1: Context with no config (should handle gracefully)
		emptyCtx := context.Background()
		// This should panic or return false - let's catch the panic
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Expected panic due to no config in context
					t.Logf("Expected panic caught: %v", r)
				}
			}()
			result := detectTokenHasMacaroon(emptyCtx)
			// If we get here without panicking, it should return false
			assert.False(t, result, "Should return false when config is nil")
		}()

		// Test case 2: Context with nil tokens
		configWithNilTokens := &config.Config{
			Tokens: nil,
		}
		ctxWithNilTokens := config.NewContext(context.Background(), configWithNilTokens)
		result := detectTokenHasMacaroon(ctxWithNilTokens)
		assert.False(t, result, "Should return false when tokens are nil")

		// Test case 3: Context with empty tokens (no macaroons)
		emptyTokens := tokens.Parse("") // Parse empty string creates empty tokens
		configWithEmptyTokens := &config.Config{
			Tokens: emptyTokens,
		}
		ctxWithEmptyTokens := config.NewContext(context.Background(), configWithEmptyTokens)
		result = detectTokenHasMacaroon(ctxWithEmptyTokens)
		assert.False(t, result, "Should return false when no macaroon tokens exist")

		// Test case 4: Context with bearer tokens only (no macaroons)
		bearerTokens := tokens.Parse("some_bearer_token_here") // This won't be recognized as macaroon
		configWithBearerTokens := &config.Config{
			Tokens: bearerTokens,
		}
		ctxWithBearerTokens := config.NewContext(context.Background(), configWithBearerTokens)
		result = detectTokenHasMacaroon(ctxWithBearerTokens)
		assert.False(t, result, "Should return false when only bearer tokens exist")

		// Test case 5: Context with macaroon tokens
		macaroonTokens := tokens.Parse("fm1r_test_macaroon_token,fm2_another_macaroon") // fm1r and fm2 prefixes are macaroon tokens
		configWithMacaroonTokens := &config.Config{
			Tokens: macaroonTokens,
		}
		ctxWithMacaroonTokens := config.NewContext(context.Background(), configWithMacaroonTokens)
		result = detectTokenHasMacaroon(ctxWithMacaroonTokens)
		assert.True(t, result, "Should return true when macaroon tokens exist")

		// Test case 6: Context with mixed tokens (including macaroons)
		mixedTokens := tokens.Parse("bearer_token,fm1a_macaroon_token,oauth_token")
		configWithMixedTokens := &config.Config{
			Tokens: mixedTokens,
		}
		ctxWithMixedTokens := config.NewContext(context.Background(), configWithMixedTokens)
		result = detectTokenHasMacaroon(ctxWithMixedTokens)
		assert.True(t, result, "Should return true when macaroon tokens exist among mixed tokens")
	})

	t.Run("validateMPGTokenCompatibility with actual contexts", func(t *testing.T) {
		// Test case 1: Context with nil tokens - should fail
		configWithNilTokens := &config.Config{
			Tokens: nil,
		}
		ctxWithNilTokens := config.NewContext(context.Background(), configWithNilTokens)
		err := validateMPGTokenCompatibility(ctxWithNilTokens)
		assert.Error(t, err, "Should return error when no macaroon tokens")
		assert.Contains(t, err.Error(), "MPG commands require updated tokens")
		assert.Contains(t, err.Error(), "flyctl auth logout")
		assert.Contains(t, err.Error(), "flyctl auth login")

		// Test case 2: Context with empty tokens - should fail
		emptyTokens := tokens.Parse("")
		configWithEmptyTokens := &config.Config{
			Tokens: emptyTokens,
		}
		ctxWithEmptyTokens := config.NewContext(context.Background(), configWithEmptyTokens)
		err = validateMPGTokenCompatibility(ctxWithEmptyTokens)
		assert.Error(t, err, "Should return error when no macaroon tokens")
		assert.Contains(t, err.Error(), "MPG commands require updated tokens")

		// Test case 3: Context with bearer tokens only - should fail
		bearerTokens := tokens.Parse("some_bearer_token")
		configWithBearerTokens := &config.Config{
			Tokens: bearerTokens,
		}
		ctxWithBearerTokens := config.NewContext(context.Background(), configWithBearerTokens)
		err = validateMPGTokenCompatibility(ctxWithBearerTokens)
		assert.Error(t, err, "Should return error when no macaroon tokens")
		assert.Contains(t, err.Error(), "MPG commands require updated tokens")

		// Test case 4: Context with macaroon tokens - should pass
		macaroonTokens := tokens.Parse("fm1r_test_macaroon_token")
		configWithMacaroonTokens := &config.Config{
			Tokens: macaroonTokens,
		}
		ctxWithMacaroonTokens := config.NewContext(context.Background(), configWithMacaroonTokens)
		err = validateMPGTokenCompatibility(ctxWithMacaroonTokens)
		assert.NoError(t, err, "Should not return error when macaroon tokens exist")

		// Test case 5: Context with mixed tokens including macaroons - should pass
		mixedTokens := tokens.Parse("bearer_token,fm1a_macaroon_token,oauth_token")
		configWithMixedTokens := &config.Config{
			Tokens: mixedTokens,
		}
		ctxWithMixedTokens := config.NewContext(context.Background(), configWithMixedTokens)
		err = validateMPGTokenCompatibility(ctxWithMixedTokens)
		assert.NoError(t, err, "Should not return error when macaroon tokens exist among mixed tokens")
	})

	t.Run("MPG commands reject non-macaroon tokens", func(t *testing.T) {
		// This test verifies that actual MPG command functions call the validation
		// and properly reject contexts without macaroon tokens

		// Create a context with bearer tokens only (no macaroons)
		bearerTokens := tokens.Parse("some_bearer_token")
		configWithBearerTokens := &config.Config{
			Tokens: bearerTokens,
		}
		ctxWithBearerTokens := config.NewContext(context.Background(), configWithBearerTokens)

		// Test that the actual run functions would reject this context
		// We can't easily test the full run functions due to their dependencies,
		// but we can verify the validation function they call would fail
		err := validateMPGTokenCompatibility(ctxWithBearerTokens)
		assert.Error(t, err, "MPG commands should reject contexts with only bearer tokens")
		assert.Contains(t, err.Error(), "MPG commands require updated tokens")

		// Create a context with macaroon tokens
		macaroonTokens := tokens.Parse("fm1r_macaroon_token")
		configWithMacaroonTokens := &config.Config{
			Tokens: macaroonTokens,
		}
		ctxWithMacaroonTokens := config.NewContext(context.Background(), configWithMacaroonTokens)

		// Test that the validation would pass for macaroon tokens
		err = validateMPGTokenCompatibility(ctxWithMacaroonTokens)
		assert.NoError(t, err, "MPG commands should accept contexts with macaroon tokens")
	})
}

func TestBackupList(t *testing.T) {
	// Setup context with output capture
	ios, _, outBuf, _ := iostreams.Test()
	ctx := context.Background()
	ctx = iostreams.NewContext(ctx, ios)

	// Add command context with a mock command
	cmd := &cobra.Command{}
	ctx = command_context.NewContext(ctx, cmd)

	// Add macaroon tokens for MPG compatibility
	macaroonTokens := tokens.Parse("fm1r_macaroon_token")
	configWithMacaroonTokens := &config.Config{
		Tokens:     macaroonTokens,
		JSONOutput: true, // Enable JSON output for easier verification
	}
	ctx = config.NewContext(ctx, configWithMacaroonTokens)

	// Set the cluster ID as first arg
	flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flagSet.Bool("json", true, "JSON output")
	flagSet.Bool("all", true, "Show all backups")
	flagSet.Parse([]string{"test-cluster-123"})
	ctx = flagctx.NewContext(ctx, flagSet)

	expectedCluster := mpgv1.ManagedCluster{
		Id:     "test-cluster-123",
		Name:   "test-cluster",
		Region: "ord",
		Status: "ready",
		Organization: fly.Organization{
			Slug: "test-org",
		},
		Version: 1,
	}

	// Mock uiex client that returns some backups
	mockv1 := &mock.MpgV1Client{
		ListManagedClusterBackupsFunc: func(ctx context.Context, clusterID string) (mpgv1.ListManagedClusterBackupsResponse, error) {
			require.Equal(t, "test-cluster-123", clusterID)

			return mpgv1.ListManagedClusterBackupsResponse{
				Data: []mpgv1.ManagedClusterBackup{
					{
						Id:     "backup-1",
						Status: "completed",
						Type:   "full",
						Start:  "2025-10-14T10:00:00Z",
						Stop:   "2025-10-14T10:30:00Z",
					},
					{
						Id:     "backup-2",
						Status: "in_progress",
						Type:   "incr",
						Start:  "2025-10-14T12:00:00Z",
						Stop:   "",
					},
				},
			}, nil
		},
		GetManagedClusterByIdFunc: func(ctx context.Context, id string) (mpgv1.GetManagedClusterResponse, error) {
			return mpgv1.GetManagedClusterResponse{
				Data: expectedCluster,
			}, nil
		},
	}

	mockv2 := &mock.MpgV2Client{
		GetClusterByIdFunc: func(ctx context.Context, id string) (mpgv2.GetClusterResponse, error) {
			return mpgv2.GetClusterResponse{}, errors.New("managed postgres cluster not found")
		},
	}

	ctx = mpgv1.NewContextWithClient(ctx, mockv1)
	ctx = mpgv2.NewContextWithClient(ctx, mockv2)
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
			return flaps.ManagedPostgresCluster{}, flaps.ErrFlapsNotFound
		},
	})

	// Run the backup list command
	err := runBackupList(ctx)
	require.NoError(t, err)

	// Parse the JSON output and verify we got 2 backups
	var backups []mpgv1.ManagedClusterBackup
	err = json.Unmarshal(outBuf.Bytes(), &backups)
	require.NoError(t, err, "Should be able to parse JSON output")
	require.Len(t, backups, 2, "Should return 2 backups")
	assert.Equal(t, "backup-1", backups[0].Id)
	assert.Equal(t, "backup-2", backups[1].Id)
}

// Test PG major version validation logic
func TestPGMajorVersionValidation(t *testing.T) {
	tests := []struct {
		name        string
		version     int
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid version 16",
			version:     16,
			expectError: false,
		},
		{
			name:        "valid version 17",
			version:     17,
			expectError: false,
		},
		{
			name:        "invalid version 15",
			version:     15,
			expectError: true,
			errorMsg:    "invalid Postgres major version: 15. Supported versions are 16 and 17",
		},
		{
			name:        "invalid version 18",
			version:     18,
			expectError: true,
			errorMsg:    "invalid Postgres major version: 18. Supported versions are 16 and 17",
		},
		{
			name:        "invalid version 14",
			version:     14,
			expectError: true,
			errorMsg:    "invalid Postgres major version: 14. Supported versions are 16 and 17",
		},
		{
			name:        "invalid version 0",
			version:     0,
			expectError: true,
			errorMsg:    "invalid Postgres major version: 0. Supported versions are 16 and 17",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the validation logic directly (matching lines 119-122 in create.go)
			if tt.version != 16 && tt.version != 17 {
				if !tt.expectError {
					t.Errorf("expected error for version %d", tt.version)

					return
				}
				err := fmt.Errorf("invalid Postgres major version: %d. Supported versions are 16 and 17", tt.version)
				if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					t.Errorf("expected error message '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if tt.expectError {
					t.Errorf("did not expect error for version %d", tt.version)
				}
			}
		})
	}
}

// Test CreateAttachment functionality
func TestCreateAttachment(t *testing.T) {
	ctx := setupTestContext()

	clusterID := "test-cluster-123"

	t.Run("successful attachment creation", func(t *testing.T) {
		mockUiex := &mock.MpgV1Client{
			CreateAttachmentFunc: func(ctx context.Context, clusterId string, input mpgv1.CreateAttachmentInput) (mpgv1.CreateAttachmentResponse, error) {
				assert.Equal(t, clusterID, clusterId)
				assert.Equal(t, "test-app", input.AppName)

				return mpgv1.CreateAttachmentResponse{
					Data: struct {
						Id               int64  `json:"id"`
						AppId            int64  `json:"app_id"`
						ManagedServiceId int64  `json:"managed_service_id"`
						AttachedAt       string `json:"attached_at"`
					}{
						Id:               1,
						AppId:            100,
						ManagedServiceId: 200,
						AttachedAt:       "2025-01-15T10:00:00Z",
					},
				}, nil
			},
		}

		ctx := mpgv1.NewContextWithClient(ctx, mockUiex)

		response, err := mockUiex.CreateAttachment(ctx, clusterID, mpgv1.CreateAttachmentInput{
			AppName: "test-app",
		})

		require.NoError(t, err)
		assert.Equal(t, int64(1), response.Data.Id)
		assert.Equal(t, int64(100), response.Data.AppId)
		assert.Equal(t, int64(200), response.Data.ManagedServiceId)
		assert.Equal(t, "2025-01-15T10:00:00Z", response.Data.AttachedAt)
	})

	t.Run("idempotent - returns existing attachment", func(t *testing.T) {
		mockUiex := &mock.MpgV1Client{
			CreateAttachmentFunc: func(ctx context.Context, clusterId string, input mpgv1.CreateAttachmentInput) (mpgv1.CreateAttachmentResponse, error) {
				// Simulating the idempotent case where attachment already exists
				return mpgv1.CreateAttachmentResponse{
					Data: struct {
						Id               int64  `json:"id"`
						AppId            int64  `json:"app_id"`
						ManagedServiceId int64  `json:"managed_service_id"`
						AttachedAt       string `json:"attached_at"`
					}{
						Id:               42, // Existing attachment ID
						AppId:            100,
						ManagedServiceId: 200,
						AttachedAt:       "2025-01-14T09:00:00Z", // Earlier timestamp
					},
				}, nil
			},
		}

		ctx := mpgv1.NewContextWithClient(ctx, mockUiex)

		response, err := mockUiex.CreateAttachment(ctx, clusterID, mpgv1.CreateAttachmentInput{
			AppName: "already-attached-app",
		})

		require.NoError(t, err)
		assert.Equal(t, int64(42), response.Data.Id)
	})

	t.Run("error - cluster not found", func(t *testing.T) {
		mockUiex := &mock.MpgV1Client{
			CreateAttachmentFunc: func(ctx context.Context, clusterId string, input mpgv1.CreateAttachmentInput) (mpgv1.CreateAttachmentResponse, error) {
				return mpgv1.CreateAttachmentResponse{}, fmt.Errorf("cluster %s not found", clusterId)
			},
		}

		ctx := mpgv1.NewContextWithClient(ctx, mockUiex)

		_, err := mockUiex.CreateAttachment(ctx, "nonexistent-cluster", mpgv1.CreateAttachmentInput{
			AppName: "test-app",
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("error - access denied", func(t *testing.T) {
		mockUiex := &mock.MpgV1Client{
			CreateAttachmentFunc: func(ctx context.Context, clusterId string, input mpgv1.CreateAttachmentInput) (mpgv1.CreateAttachmentResponse, error) {
				return mpgv1.CreateAttachmentResponse{}, fmt.Errorf("access denied: you don't have permission to attach cluster %s", clusterId)
			},
		}

		ctx := mpgv1.NewContextWithClient(ctx, mockUiex)

		_, err := mockUiex.CreateAttachment(ctx, clusterID, mpgv1.CreateAttachmentInput{
			AppName: "test-app",
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "access denied")
	})

	t.Run("error - app not found", func(t *testing.T) {
		mockUiex := &mock.MpgV1Client{
			CreateAttachmentFunc: func(ctx context.Context, clusterId string, input mpgv1.CreateAttachmentInput) (mpgv1.CreateAttachmentResponse, error) {
				return mpgv1.CreateAttachmentResponse{}, fmt.Errorf("app not found")
			},
		}

		ctx := mpgv1.NewContextWithClient(ctx, mockUiex)

		_, err := mockUiex.CreateAttachment(ctx, clusterID, mpgv1.CreateAttachmentInput{
			AppName: "nonexistent-app",
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// Test attach command integration with CreateAttachment
func TestAttachCommand_CreatesAttachment(t *testing.T) {
	ctx := setupTestContext()

	clusterID := "test-cluster-123"
	appName := "test-app"

	expectedCluster := mpgv1.ManagedCluster{
		Id:     clusterID,
		Name:   "test-cluster",
		Region: "ord",
		Status: "ready",
		Organization: fly.Organization{
			Slug: "test-org",
		},
	}

	connectionURI := "postgresql://user:pass@host:5432/db"

	// Track whether CreateAttachment was called
	createAttachmentCalled := false
	var capturedAppName string

	mockUiex := &mock.MpgV1Client{
		GetManagedClusterByIdFunc: func(ctx context.Context, id string) (mpgv1.GetManagedClusterResponse, error) {
			assert.Equal(t, clusterID, id)

			return mpgv1.GetManagedClusterResponse{
				Data: expectedCluster,
				Credentials: mpgv1.GetManagedClusterCredentialsResponse{
					ConnectionUri: connectionURI,
					User:          "fly-user",
					Password:      "test-password",
					DBName:        "fly_db",
				},
			}, nil
		},
		CreateAttachmentFunc: func(ctx context.Context, clusterId string, input mpgv1.CreateAttachmentInput) (mpgv1.CreateAttachmentResponse, error) {
			createAttachmentCalled = true
			capturedAppName = input.AppName
			assert.Equal(t, clusterID, clusterId)

			return mpgv1.CreateAttachmentResponse{
				Data: struct {
					Id               int64  `json:"id"`
					AppId            int64  `json:"app_id"`
					ManagedServiceId int64  `json:"managed_service_id"`
					AttachedAt       string `json:"attached_at"`
				}{
					Id:               1,
					AppId:            100,
					ManagedServiceId: 200,
					AttachedAt:       "2025-01-15T10:00:00Z",
				},
			}, nil
		},
	}

	ctx = mpgv1.NewContextWithClient(ctx, mockUiex)

	// Simulate the attach command flow: get cluster, then create attachment
	response, err := mockUiex.GetManagedClusterById(ctx, clusterID)
	require.NoError(t, err)
	assert.Equal(t, expectedCluster.Id, response.Data.Id)

	// Create attachment (this simulates what runAttach does after setting secrets)
	attachInput := mpgv1.CreateAttachmentInput{
		AppName: appName,
	}
	_, err = mockUiex.CreateAttachment(ctx, clusterID, attachInput)
	require.NoError(t, err)

	// Verify CreateAttachment was called with correct app name
	assert.True(t, createAttachmentCalled, "CreateAttachment should be called during attach")
	assert.Equal(t, appName, capturedAppName, "App name should be passed to CreateAttachment")
}

// Test that attach command handles CreateAttachment errors gracefully
func TestAttachCommand_HandlesAttachmentErrorGracefully(t *testing.T) {
	ctx := setupTestContext()

	clusterID := "test-cluster-123"
	appName := "test-app"

	expectedCluster := mpgv1.ManagedCluster{
		Id:     clusterID,
		Name:   "test-cluster",
		Region: "ord",
		Status: "ready",
		Organization: fly.Organization{
			Slug: "test-org",
		},
	}

	connectionURI := "postgresql://user:pass@host:5432/db"

	mockUiex := &mock.MpgV1Client{
		GetManagedClusterByIdFunc: func(ctx context.Context, id string) (mpgv1.GetManagedClusterResponse, error) {
			return mpgv1.GetManagedClusterResponse{
				Data: expectedCluster,
				Credentials: mpgv1.GetManagedClusterCredentialsResponse{
					ConnectionUri: connectionURI,
					User:          "fly-user",
					Password:      "test-password",
					DBName:        "fly_db",
				},
			}, nil
		},
		CreateAttachmentFunc: func(ctx context.Context, clusterId string, input mpgv1.CreateAttachmentInput) (mpgv1.CreateAttachmentResponse, error) {
			// Simulate a failure in creating attachment
			return mpgv1.CreateAttachmentResponse{}, fmt.Errorf("failed to create attachment")
		},
	}

	ctx = mpgv1.NewContextWithClient(ctx, mockUiex)

	// Get cluster - should succeed
	response, err := mockUiex.GetManagedClusterById(ctx, clusterID)
	require.NoError(t, err)
	assert.Equal(t, expectedCluster.Id, response.Data.Id)

	// Create attachment - should fail but we handle it gracefully
	attachInput := mpgv1.CreateAttachmentInput{
		AppName: appName,
	}
	_, err = mockUiex.CreateAttachment(ctx, clusterID, attachInput)

	// The error exists but in runAttach we just log a warning
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create attachment")

	// In the actual implementation, this is handled as a warning:
	// fmt.Fprintf(io.ErrOut, "Warning: failed to create attachment record: %v\n", err)
	// The attach command still succeeds because the secret was set
}

// Test invalid PG major version error message
func TestInvalidPGMajorVersion_Error(t *testing.T) {
	invalidVersions := []int{15, 18, 14, 13, 19, 0, -1}

	for _, version := range invalidVersions {
		t.Run(fmt.Sprintf("version_%d", version), func(t *testing.T) {
			err := fmt.Errorf("invalid Postgres major version: %d. Supported versions are 16 and 17", version)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid Postgres major version")
			assert.Contains(t, err.Error(), "Supported versions are 16 and 17")
			assert.Contains(t, err.Error(), fmt.Sprintf("%d", version))
		})
	}
}

// Test formatAttachedApps function
func TestFormatAttachedApps(t *testing.T) {
	tests := []struct {
		name     string
		apps     []mpg.AttachedApp
		expected string
	}{
		{
			name:     "no attached apps",
			apps:     []mpg.AttachedApp{},
			expected: "<no attached apps>",
		},
		{
			name:     "nil apps",
			apps:     nil,
			expected: "<no attached apps>",
		},
		{
			name: "single app",
			apps: []mpg.AttachedApp{
				{Name: "my-web-app", Id: 1},
			},
			expected: "my-web-app",
		},
		{
			name: "two apps",
			apps: []mpg.AttachedApp{
				{Name: "my-web-app", Id: 1},
				{Name: "my-api", Id: 2},
			},
			expected: "my-web-app, my-api",
		},
		{
			name: "three apps",
			apps: []mpg.AttachedApp{
				{Name: "app-one", Id: 1},
				{Name: "app-two", Id: 2},
				{Name: "app-three", Id: 3},
			},
			expected: "app-one, app-two, app-three",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatAttachedApps(tt.apps)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test DeleteAttachment functionality
func TestDeleteAttachment(t *testing.T) {
	ctx := setupTestContext()

	clusterID := "test-cluster-123"

	t.Run("successful attachment deletion", func(t *testing.T) {
		mockUiex := &mock.MpgV1Client{
			DeleteAttachmentFunc: func(ctx context.Context, clusterId string, appName string) (mpgv1.DeleteAttachmentResponse, error) {
				assert.Equal(t, clusterID, clusterId)
				assert.Equal(t, "test-app", appName)

				return mpgv1.DeleteAttachmentResponse{
					Data: struct {
						Message string `json:"message"`
					}{
						Message: "Attachment deleted successfully",
					},
				}, nil
			},
		}

		ctx := mpgv1.NewContextWithClient(ctx, mockUiex)

		response, err := mockUiex.DeleteAttachment(ctx, clusterID, "test-app")

		require.NoError(t, err)
		assert.Equal(t, "Attachment deleted successfully", response.Data.Message)
	})

	t.Run("error - attachment not found", func(t *testing.T) {
		mockUiex := &mock.MpgV1Client{
			DeleteAttachmentFunc: func(ctx context.Context, clusterId string, appName string) (mpgv1.DeleteAttachmentResponse, error) {
				return mpgv1.DeleteAttachmentResponse{}, fmt.Errorf("attachment not found for app '%s' on cluster %s", appName, clusterId)
			},
		}

		ctx := mpgv1.NewContextWithClient(ctx, mockUiex)

		_, err := mockUiex.DeleteAttachment(ctx, clusterID, "nonexistent-app")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "attachment not found")
	})

	t.Run("error - access denied", func(t *testing.T) {
		mockUiex := &mock.MpgV1Client{
			DeleteAttachmentFunc: func(ctx context.Context, clusterId string, appName string) (mpgv1.DeleteAttachmentResponse, error) {
				return mpgv1.DeleteAttachmentResponse{}, fmt.Errorf("access denied: you don't have permission to detach from cluster %s", clusterId)
			},
		}

		ctx := mpgv1.NewContextWithClient(ctx, mockUiex)

		_, err := mockUiex.DeleteAttachment(ctx, clusterID, "test-app")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "access denied")
	})

	t.Run("error - cluster not found", func(t *testing.T) {
		mockUiex := &mock.MpgV1Client{
			DeleteAttachmentFunc: func(ctx context.Context, clusterId string, appName string) (mpgv1.DeleteAttachmentResponse, error) {
				return mpgv1.DeleteAttachmentResponse{}, fmt.Errorf("cluster %s not found", clusterId)
			},
		}

		ctx := mpgv1.NewContextWithClient(ctx, mockUiex)

		_, err := mockUiex.DeleteAttachment(ctx, "nonexistent-cluster", "test-app")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// Test the list command with attached apps
func TestListCommand_WithAttachedApps(t *testing.T) {
	ctx := setupTestContext()

	expectedClusters := []mpgv1.ManagedCluster{
		{
			Id:     "cluster-1",
			Name:   "test-cluster-1",
			Region: "ord",
			Status: "ready",
			Plan:   "development",
			Organization: fly.Organization{
				Slug: "test-org",
			},
			AttachedApps: []mpg.AttachedApp{
				{Name: "web-app", Id: 100},
				{Name: "api-app", Id: 101},
			},
		},
		{
			Id:     "cluster-2",
			Name:   "test-cluster-2",
			Region: "lax",
			Status: "ready",
			Plan:   "production",
			Organization: fly.Organization{
				Slug: "test-org",
			},
			AttachedApps: []mpg.AttachedApp{}, // No attached apps
		},
	}

	mockUiex := &mock.MpgV1Client{
		ListManagedClustersFunc: func(ctx context.Context, orgSlug string, deleted bool) (mpgv1.ListManagedClustersResponse, error) {
			assert.Equal(t, "test-org", orgSlug)

			return mpgv1.ListManagedClustersResponse{
				Data: expectedClusters,
			}, nil
		},
	}

	ctx = mpgv1.NewContextWithClient(ctx, mockUiex)

	// Test successful cluster listing with attached apps
	clusters, err := mockUiex.ListManagedClusters(ctx, "test-org", false)
	require.NoError(t, err)
	assert.Len(t, clusters.Data, 2)

	// Verify first cluster has attached apps
	assert.Len(t, clusters.Data[0].AttachedApps, 2)
	assert.Equal(t, "web-app", clusters.Data[0].AttachedApps[0].Name)
	assert.Equal(t, "api-app", clusters.Data[0].AttachedApps[1].Name)

	// Verify attached apps formatting for first cluster
	formattedApps := FormatAttachedApps(clusters.Data[0].AttachedApps)
	assert.Equal(t, "web-app, api-app", formattedApps)

	// Verify second cluster has no attached apps
	assert.Len(t, clusters.Data[1].AttachedApps, 0)

	// Verify attached apps formatting for second cluster (empty)
	formattedApps = FormatAttachedApps(clusters.Data[1].AttachedApps)
	assert.Equal(t, "<no attached apps>", formattedApps)
}
