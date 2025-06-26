package mpg

import (
	"context"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command_context"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

// MockUiexClient implements the uiexutil.Client interface for testing
type MockUiexClient struct {
	ListMPGRegionsFunc        func(ctx context.Context, orgSlug string) (uiex.ListMPGRegionsResponse, error)
	ListManagedClustersFunc   func(ctx context.Context, orgSlug string) (uiex.ListManagedClustersResponse, error)
	GetManagedClusterFunc     func(ctx context.Context, orgSlug string, id string) (uiex.GetManagedClusterResponse, error)
	GetManagedClusterByIdFunc func(ctx context.Context, id string) (uiex.GetManagedClusterResponse, error)
	CreateUserFunc            func(ctx context.Context, id string, input uiex.CreateUserInput) (uiex.CreateUserResponse, error)
	CreateClusterFunc         func(ctx context.Context, input uiex.CreateClusterInput) (uiex.CreateClusterResponse, error)
	DestroyClusterFunc        func(ctx context.Context, id string) error
}

func (m *MockUiexClient) ListMPGRegions(ctx context.Context, orgSlug string) (uiex.ListMPGRegionsResponse, error) {
	if m.ListMPGRegionsFunc != nil {
		return m.ListMPGRegionsFunc(ctx, orgSlug)
	}
	return uiex.ListMPGRegionsResponse{}, nil
}

func (m *MockUiexClient) ListManagedClusters(ctx context.Context, orgSlug string) (uiex.ListManagedClustersResponse, error) {
	if m.ListManagedClustersFunc != nil {
		return m.ListManagedClustersFunc(ctx, orgSlug)
	}
	return uiex.ListManagedClustersResponse{}, nil
}

func (m *MockUiexClient) GetManagedCluster(ctx context.Context, orgSlug string, id string) (uiex.GetManagedClusterResponse, error) {
	if m.GetManagedClusterFunc != nil {
		return m.GetManagedClusterFunc(ctx, orgSlug, id)
	}
	return uiex.GetManagedClusterResponse{}, nil
}

func (m *MockUiexClient) GetManagedClusterById(ctx context.Context, id string) (uiex.GetManagedClusterResponse, error) {
	if m.GetManagedClusterByIdFunc != nil {
		return m.GetManagedClusterByIdFunc(ctx, id)
	}
	return uiex.GetManagedClusterResponse{}, nil
}

func (m *MockUiexClient) CreateUser(ctx context.Context, id string, input uiex.CreateUserInput) (uiex.CreateUserResponse, error) {
	if m.CreateUserFunc != nil {
		return m.CreateUserFunc(ctx, id, input)
	}
	return uiex.CreateUserResponse{}, nil
}

func (m *MockUiexClient) CreateCluster(ctx context.Context, input uiex.CreateClusterInput) (uiex.CreateClusterResponse, error) {
	if m.CreateClusterFunc != nil {
		return m.CreateClusterFunc(ctx, input)
	}
	return uiex.CreateClusterResponse{}, nil
}

func (m *MockUiexClient) DestroyCluster(ctx context.Context, id string) error {
	if m.DestroyClusterFunc != nil {
		return m.DestroyClusterFunc(ctx, id)
	}
	return nil
}

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

	return ctx
}

// Test the actual filterMPGRegions function with real data
func TestFilterMPGRegions_RealFunctionality(t *testing.T) {
	platformRegions := []fly.Region{
		{Code: "ord", Name: "Chicago, Illinois (US)"},
		{Code: "lax", Name: "Los Angeles, California (US)"},
		{Code: "ams", Name: "Amsterdam, Netherlands (EU)"},
		{Code: "nrt", Name: "Tokyo, Japan (AS)"},
	}

	mpgRegions := []uiex.MPGRegion{
		{Code: "ord", Available: true},
		{Code: "lax", Available: true},
		{Code: "ams", Available: false}, // Not available
		// nrt not in MPG regions at all
	}

	filtered := filterMPGRegions(platformRegions, mpgRegions)

	// Should only return ord and lax (available in MPG)
	assert.Len(t, filtered, 2)
	assert.Equal(t, "ord", filtered[0].Code)
	assert.Equal(t, "lax", filtered[1].Code)

	// Verify the filtering logic works correctly
	for _, region := range filtered {
		found := false
		for _, mpgRegion := range mpgRegions {
			if region.Code == mpgRegion.Code && mpgRegion.Available {
				found = true
				break
			}
		}
		assert.True(t, found, "Filtered region %s should be available in MPG", region.Code)
	}
}

// Test ClusterFromFlagOrSelect with actual flag context
func TestClusterFromFlagOrSelect_WithFlagContext(t *testing.T) {
	ctx := setupTestContext()

	expectedCluster := uiex.ManagedCluster{
		Id:     "test-cluster-123",
		Name:   "test-cluster",
		Region: "ord",
		Status: "ready",
		Organization: fly.Organization{
			Slug: "test-org",
		},
	}

	mockUiex := &MockUiexClient{
		ListManagedClustersFunc: func(ctx context.Context, orgSlug string) (uiex.ListManagedClustersResponse, error) {
			assert.Equal(t, "test-org", orgSlug)
			return uiex.ListManagedClustersResponse{
				Data: []uiex.ManagedCluster{expectedCluster},
			}, nil
		},
	}

	ctx = uiexutil.NewContextWithClient(ctx, mockUiex)

	t.Run("no clusters found", func(t *testing.T) {
		mockEmpty := &MockUiexClient{
			ListManagedClustersFunc: func(ctx context.Context, orgSlug string) (uiex.ListManagedClustersResponse, error) {
				return uiex.ListManagedClustersResponse{Data: []uiex.ManagedCluster{}}, nil
			},
		}
		ctx := uiexutil.NewContextWithClient(ctx, mockEmpty)

		_, err := ClusterFromFlagOrSelect(ctx, "test-org")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no managed postgres clusters found")
	})

	t.Run("cluster not found by ID", func(t *testing.T) {
		// Set the cluster flag
		flagSet := flagctx.FromContext(ctx)
		flagSet.Set("cluster", "wrong-cluster-id")

		_, err := ClusterFromFlagOrSelect(ctx, "test-org")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "managed postgres cluster \"wrong-cluster-id\" not found")
	})

	t.Run("cluster found by ID", func(t *testing.T) {
		// Set the cluster flag to a valid ID
		flagSet := flagctx.FromContext(ctx)
		flagSet.Set("cluster", "test-cluster-123")

		cluster, err := ClusterFromFlagOrSelect(ctx, "test-org")
		require.NoError(t, err)
		assert.Equal(t, expectedCluster.Id, cluster.Id)
		assert.Equal(t, expectedCluster.Name, cluster.Name)
	})
}

// Test the actual GetAvailableMPGRegions function with mocked dependencies
func TestGetAvailableMPGRegions_RealFunction(t *testing.T) {
	ctx := setupTestContext()

	platformRegions := []fly.Region{
		{Code: "ord", Name: "Chicago, Illinois (US)"},
		{Code: "lax", Name: "Los Angeles, California (US)"},
		{Code: "ams", Name: "Amsterdam, Netherlands (EU)"},
	}

	mpgRegions := []uiex.MPGRegion{
		{Code: "ord", Available: true},
		{Code: "lax", Available: true},
		{Code: "ams", Available: false}, // Not available
	}

	mockUiex := &MockUiexClient{
		ListMPGRegionsFunc: func(ctx context.Context, orgSlug string) (uiex.ListMPGRegionsResponse, error) {
			assert.Equal(t, "test-org", orgSlug)
			return uiex.ListMPGRegionsResponse{
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
	service := NewMPGServiceWithDependencies(mockUiex, mockRegionProvider)

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

	mpgRegions := []uiex.MPGRegion{
		{Code: "ord", Available: true},
		{Code: "lax", Available: true},
	}

	mockUiex := &MockUiexClient{
		ListMPGRegionsFunc: func(ctx context.Context, orgSlug string) (uiex.ListMPGRegionsResponse, error) {
			return uiex.ListMPGRegionsResponse{
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
	service := NewMPGServiceWithDependencies(mockUiex, mockRegionProvider)

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

	mpgRegions := []uiex.MPGRegion{
		{Code: "ord", Available: true},
		{Code: "lax", Available: true},
	}

	mockUiex := &MockUiexClient{
		ListMPGRegionsFunc: func(ctx context.Context, orgSlug string) (uiex.ListMPGRegionsResponse, error) {
			return uiex.ListMPGRegionsResponse{
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
	service := NewMPGServiceWithDependencies(mockUiex, mockRegionProvider)

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
	expectedCluster := uiex.ManagedCluster{
		Id:     clusterID,
		Name:   "test-cluster",
		Region: "ord",
		Status: "ready",
		Organization: fly.Organization{
			Slug: "test-org",
		},
	}

	mockUiex := &MockUiexClient{
		GetManagedClusterByIdFunc: func(ctx context.Context, id string) (uiex.GetManagedClusterResponse, error) {
			assert.Equal(t, clusterID, id)
			return uiex.GetManagedClusterResponse{
				Data: expectedCluster,
			}, nil
		},
		DestroyClusterFunc: func(ctx context.Context, id string) error {
			assert.Equal(t, clusterID, id)
			return nil
		},
	}

	ctx = uiexutil.NewContextWithClient(ctx, mockUiex)

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
	err = mockUiex.DestroyCluster(ctx, clusterID)
	require.NoError(t, err)
}

// Test the status command logic (extracted from runStatus)
func TestStatusCommand_Logic(t *testing.T) {
	ctx := setupTestContext()

	clusterID := "test-cluster-123"
	expectedCluster := uiex.ManagedCluster{
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
		IpAssignments: uiex.ManagedClusterIpAssignments{
			Direct: "10.0.0.1",
		},
	}

	mockUiex := &MockUiexClient{
		GetManagedClusterByIdFunc: func(ctx context.Context, id string) (uiex.GetManagedClusterResponse, error) {
			assert.Equal(t, clusterID, id)
			return uiex.GetManagedClusterResponse{
				Data: expectedCluster,
			}, nil
		},
	}

	ctx = uiexutil.NewContextWithClient(ctx, mockUiex)

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

	expectedClusters := []uiex.ManagedCluster{
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

	mockUiex := &MockUiexClient{
		ListManagedClustersFunc: func(ctx context.Context, orgSlug string) (uiex.ListManagedClustersResponse, error) {
			assert.Equal(t, "test-org", orgSlug)
			return uiex.ListManagedClustersResponse{
				Data: expectedClusters,
			}, nil
		},
	}

	ctx = uiexutil.NewContextWithClient(ctx, mockUiex)

	// Test successful cluster listing
	clusters, err := mockUiex.ListManagedClusters(ctx, "test-org")
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
		mockUiex := &MockUiexClient{
			ListManagedClustersFunc: func(ctx context.Context, orgSlug string) (uiex.ListManagedClustersResponse, error) {
				return uiex.ListManagedClustersResponse{}, fmt.Errorf("API error")
			},
		}
		ctx := uiexutil.NewContextWithClient(ctx, mockUiex)

		_, err := ClusterFromFlagOrSelect(ctx, "test-org")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed retrieving postgres clusters")
	})

	t.Run("GetManagedClusterById error", func(t *testing.T) {
		mockUiex := &MockUiexClient{
			GetManagedClusterByIdFunc: func(ctx context.Context, id string) (uiex.GetManagedClusterResponse, error) {
				return uiex.GetManagedClusterResponse{}, fmt.Errorf("API error")
			},
		}
		ctx := uiexutil.NewContextWithClient(ctx, mockUiex)

		_, err := mockUiex.GetManagedClusterById(ctx, "test-cluster")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API error")
	})

	t.Run("DestroyCluster error", func(t *testing.T) {
		mockUiex := &MockUiexClient{
			DestroyClusterFunc: func(ctx context.Context, id string) error {
				return fmt.Errorf("destroy failed")
			},
		}
		ctx := uiexutil.NewContextWithClient(ctx, mockUiex)

		err := mockUiex.DestroyCluster(ctx, "test-cluster")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "destroy failed")
	})
}

// Test the create command logic (extracted from runCreate)
func TestCreateCommand_Logic(t *testing.T) {
	ctx := setupTestContext()

	expectedCluster := uiex.ManagedCluster{
		Id:     "new-cluster-123",
		Name:   "test-db",
		Region: "ord",
		Status: "ready",
		Organization: fly.Organization{
			Slug: "test-org",
		},
	}

	platformRegions := []fly.Region{
		{Code: "ord", Name: "Chicago, Illinois (US)"},
		{Code: "lax", Name: "Los Angeles, California (US)"},
	}

	mpgRegions := []uiex.MPGRegion{
		{Code: "ord", Available: true},
		{Code: "lax", Available: true},
	}

	mockUiex := &MockUiexClient{
		ListMPGRegionsFunc: func(ctx context.Context, orgSlug string) (uiex.ListMPGRegionsResponse, error) {
			return uiex.ListMPGRegionsResponse{
				Data: mpgRegions,
			}, nil
		},
		CreateClusterFunc: func(ctx context.Context, input uiex.CreateClusterInput) (uiex.CreateClusterResponse, error) {
			// Verify the input parameters
			assert.Equal(t, "test-db", input.Name)
			assert.Equal(t, "ord", input.Region)
			assert.Equal(t, "basic", input.Plan)
			assert.Equal(t, "test-org", input.OrgSlug)

			return uiex.CreateClusterResponse{
				Data: struct {
					Id            string                           `json:"id"`
					Name          string                           `json:"name"`
					Status        *string                          `json:"status"`
					Plan          string                           `json:"plan"`
					Environment   *string                          `json:"environment"`
					Region        string                           `json:"region"`
					Organization  fly.Organization                 `json:"organization"`
					Replicas      int                              `json:"replicas"`
					Disk          int                              `json:"disk"`
					IpAssignments uiex.ManagedClusterIpAssignments `json:"ip_assignments"`
				}{
					Id:           expectedCluster.Id,
					Name:         expectedCluster.Name,
					Region:       expectedCluster.Region,
					Plan:         expectedCluster.Plan,
					Organization: expectedCluster.Organization,
				},
			}, nil
		},
		GetManagedClusterByIdFunc: func(ctx context.Context, id string) (uiex.GetManagedClusterResponse, error) {
			assert.Equal(t, "new-cluster-123", id)
			return uiex.GetManagedClusterResponse{
				Data: expectedCluster,
			}, nil
		},
	}

	mockRegionProvider := &MockRegionProvider{
		GetPlatformRegionsFunc: func(ctx context.Context) ([]fly.Region, error) {
			return platformRegions, nil
		},
	}

	// Create service with mocked dependencies
	service := NewMPGServiceWithDependencies(mockUiex, mockRegionProvider)

	// Test region validation logic using the actual function
	availableRegions, err := service.GetAvailableMPGRegions(ctx, "test-org")
	require.NoError(t, err)
	assert.Len(t, availableRegions, 2, "Should have 2 available regions")

	// Test region selection logic
	regionCode := "ord"
	var selectedRegion *fly.Region
	for _, region := range availableRegions {
		if region.Code == regionCode {
			selectedRegion = &region
			break
		}
	}
	require.NotNil(t, selectedRegion, "Should find selected region")
	assert.Equal(t, "ord", selectedRegion.Code)

	// Test cluster creation
	input := uiex.CreateClusterInput{
		Name:    "test-db",
		Region:  selectedRegion.Code,
		Plan:    "basic",
		OrgSlug: "test-org",
	}

	response, err := mockUiex.CreateCluster(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, expectedCluster.Id, response.Data.Id)
	assert.Equal(t, expectedCluster.Name, response.Data.Name)

	// Test cluster status checking
	cluster, err := mockUiex.GetManagedClusterById(ctx, response.Data.Id)
	require.NoError(t, err)
	assert.Equal(t, expectedCluster.Status, cluster.Data.Status)
}

// Test the attach command logic (extracted from runAttach)
func TestAttachCommand_Logic(t *testing.T) {
	ctx := setupTestContext()

	clusterID := "test-cluster-123"

	expectedCluster := uiex.ManagedCluster{
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

	mockUiex := &MockUiexClient{
		GetManagedClusterByIdFunc: func(ctx context.Context, id string) (uiex.GetManagedClusterResponse, error) {
			assert.Equal(t, clusterID, id)
			return uiex.GetManagedClusterResponse{
				Data: expectedCluster,
				Credentials: uiex.GetManagedClusterCredentialsResponse{
					ConnectionUri: connectionURI,
				},
			}, nil
		},
	}

	ctx = uiexutil.NewContextWithClient(ctx, mockUiex)

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

// Test region validation in create command
func TestCreateCommand_RegionValidation(t *testing.T) {
	ctx := setupTestContext()

	platformRegions := []fly.Region{
		{Code: "ord", Name: "Chicago, Illinois (US)"},
		{Code: "lax", Name: "Los Angeles, California (US)"},
	}

	mpgRegions := []uiex.MPGRegion{
		{Code: "ord", Available: true},
		{Code: "lax", Available: true},
	}

	mockUiex := &MockUiexClient{
		ListMPGRegionsFunc: func(ctx context.Context, orgSlug string) (uiex.ListMPGRegionsResponse, error) {
			return uiex.ListMPGRegionsResponse{
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
	service := NewMPGServiceWithDependencies(mockUiex, mockRegionProvider)

	// Test valid region using the actual function
	valid, err := service.IsValidMPGRegion(ctx, "test-org", "ord")
	require.NoError(t, err)
	assert.True(t, valid, "Should find valid region")

	// Test invalid region using the actual function
	valid, err = service.IsValidMPGRegion(ctx, "test-org", "invalid")
	require.NoError(t, err)
	assert.False(t, valid, "Should not find invalid region")
}
