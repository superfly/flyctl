package plan

import (
	"context"
	"testing"

	genq "github.com/Khan/genqlient/graphql"
	"github.com/spf13/pflag"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

// mockUIEXClient implements uiexutil.Client for testing
type mockUIEXClient struct {
	mpgRegions []uiex.MPGRegion
}

func (m *mockUIEXClient) ListMPGRegions(ctx context.Context, orgSlug string) (uiex.ListMPGRegionsResponse, error) {
	return uiex.ListMPGRegionsResponse{Data: m.mpgRegions}, nil
}

// mockGenqClient implements the genq.Client interface for testing
type mockGenqClient struct{}

func (m *mockGenqClient) MakeRequest(ctx context.Context, req *genq.Request, resp *genq.Response) error {
	// Mock the GetOrganization response - just return the same slug
	// This simulates the ResolveOrganizationSlug behavior
	resp.Data = map[string]interface{}{
		"organization": map[string]interface{}{
			"rawSlug": "test-org", // Return a fixed value for testing
		},
	}
	return nil
}

func (m *mockUIEXClient) ListManagedClusters(ctx context.Context, orgSlug string) (uiex.ListManagedClustersResponse, error) {
	return uiex.ListManagedClustersResponse{}, nil
}

func (m *mockUIEXClient) GetManagedCluster(ctx context.Context, orgSlug string, id string) (uiex.GetManagedClusterResponse, error) {
	return uiex.GetManagedClusterResponse{}, nil
}

func (m *mockUIEXClient) GetManagedClusterById(ctx context.Context, id string) (uiex.GetManagedClusterResponse, error) {
	return uiex.GetManagedClusterResponse{}, nil
}

func (m *mockUIEXClient) CreateUser(ctx context.Context, id string, input uiex.CreateUserInput) (uiex.CreateUserResponse, error) {
	return uiex.CreateUserResponse{}, nil
}

func (m *mockUIEXClient) CreateCluster(ctx context.Context, input uiex.CreateClusterInput) (uiex.CreateClusterResponse, error) {
	return uiex.CreateClusterResponse{}, nil
}

func (m *mockUIEXClient) DestroyCluster(ctx context.Context, orgSlug string, id string) error {
	return nil
}

func (m *mockUIEXClient) CreateFlyManagedBuilder(ctx context.Context, orgSlug string, region string) (uiex.CreateFlyManagedBuilderResponse, error) {
	return uiex.CreateFlyManagedBuilderResponse{}, nil
}

func (m *mockUIEXClient) CreateDeploy(ctx context.Context, appName string, input uiex.RemoteDeploymentRequest) (uiex.RemoteDeploymentResponse, error) {
	return uiex.RemoteDeploymentResponse{}, nil
}

func TestDefaultPostgres_ForceTypes(t *testing.T) {
	tests := []struct {
		name              string
		dbFlag            string
		mpgEnabled        bool
		mpgRegionsWithIAD bool   // whether iad region supports MPG
		expectedType      string // "managed", "unmanaged", or "default"
		expectError       bool
	}{
		{
			name:              "force managed postgres with region support",
			dbFlag:            "mpg",
			mpgEnabled:        true,
			mpgRegionsWithIAD: true,
			expectedType:      "managed",
			expectError:       false,
		},
		{
			name:              "force unmanaged postgres",
			dbFlag:            "upg",
			mpgEnabled:        true,
			mpgRegionsWithIAD: true,
			expectedType:      "unmanaged",
			expectError:       false,
		},
		{
			name:              "force legacy postgres",
			dbFlag:            "legacy",
			mpgEnabled:        true,
			mpgRegionsWithIAD: true,
			expectedType:      "unmanaged",
			expectError:       false,
		},
		{
			name:              "default non-interactive behavior with mpg enabled and region support",
			dbFlag:            "true",
			mpgEnabled:        true,
			mpgRegionsWithIAD: true,
			expectedType:      "unmanaged",
			expectError:       false,
		},
		{
			name:              "default non-interactive behavior with mpg enabled but no region support",
			dbFlag:            "true",
			mpgEnabled:        true,
			mpgRegionsWithIAD: false,
			expectedType:      "unmanaged",
			expectError:       false,
		},
		{
			name:              "default behavior with mpg disabled",
			dbFlag:            "true",
			mpgEnabled:        false,
			mpgRegionsWithIAD: false,
			expectedType:      "unmanaged",
			expectError:       false,
		},
		{
			name:              "force unmanaged overrides mpg enabled",
			dbFlag:            "upg",
			mpgEnabled:        true,
			mpgRegionsWithIAD: true,
			expectedType:      "unmanaged",
			expectError:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a context with iostreams
			ctx := context.Background()
			ctx = iostreams.NewContext(ctx, iostreams.System())

			// Create a test context with flags
			flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flagSet.String("db", tt.dbFlag, "")
			ctx = flagctx.NewContext(ctx, flagSet)

			// Set up mock UIEX client for MPG regions
			var mpgRegions []uiex.MPGRegion
			if tt.mpgRegionsWithIAD {
				mpgRegions = []uiex.MPGRegion{
					{Code: "iad", Available: true},
					{Code: "lax", Available: true},
				}
			} else {
				mpgRegions = []uiex.MPGRegion{
					{Code: "lax", Available: true},
					{Code: "fra", Available: true},
					// iad is not in the list, so it's not available
				}
			}
			mockUIEX := &mockUIEXClient{mpgRegions: mpgRegions}
			ctx = uiexutil.NewContextWithClient(ctx, mockUIEX)

			// Set up mock API client for platform regions
			mockClient := &mock.Client{
				PlatformRegionsFunc: func(ctx context.Context) ([]fly.Region, *fly.Region, error) {
					// Return some mock regions for testing
					return []fly.Region{
						{Code: "iad", Name: "Ashburn, Virginia (US)"},
						{Code: "lax", Name: "Los Angeles, California (US)"},
						{Code: "fra", Name: "Frankfurt, Germany"},
					}, &fly.Region{Code: "iad", Name: "Ashburn, Virginia (US)"}, nil
				},
				GenqClientFunc: func() genq.Client {
					return &mockGenqClient{}
				},
			}
			ctx = flyutil.NewContextWithClient(ctx, mockClient)

			// Create a mock launch plan
			plan := &LaunchPlan{
				AppName:    "test-app",
				OrgSlug:    "test-org",
				RegionCode: "iad", // Use iad region for testing
			}

			result, err := DefaultPostgres(ctx, plan, tt.mpgEnabled)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
					return
				}
			}

			// Check the type of postgres plan returned
			switch tt.expectedType {
			case "managed":
				if result.ManagedPostgres == nil {
					t.Errorf("expected managed postgres plan but got nil")
				}
				if result.FlyPostgres != nil {
					t.Errorf("expected no fly postgres plan but got one")
				}
			case "unmanaged":
				if result.FlyPostgres == nil {
					t.Errorf("expected fly postgres plan but got nil")
				}
				if result.ManagedPostgres != nil {
					t.Errorf("expected no managed postgres plan but got one")
				}
			}
		})
	}
}

// TestDefaultPostgres_RegionSwitching tests that when MPG region switching occurs,
// the overall LaunchPlan.RegionCode is updated, not just the postgres plan
func TestDefaultPostgres_RegionSwitching(t *testing.T) {
	t.Run("region switching updates overall app region", func(t *testing.T) {
		// Create a context with iostreams (non-interactive to avoid prompts)
		ctx := context.Background()
		ctx = iostreams.NewContext(ctx, iostreams.System())

		// Create a test context with default db flag
		flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
		flagSet.String("db", "true", "")
		ctx = flagctx.NewContext(ctx, flagSet)

		// Set up mock UIEX client where iad doesn't support MPG but lax does
		mpgRegions := []uiex.MPGRegion{
			{Code: "lax", Available: true},
			{Code: "fra", Available: true},
			// iad is not in the list, so it's not available
		}
		mockUIEX := &mockUIEXClient{mpgRegions: mpgRegions}
		ctx = uiexutil.NewContextWithClient(ctx, mockUIEX)

		// Set up mock API client for platform regions
		mockClient := &mock.Client{
			PlatformRegionsFunc: func(ctx context.Context) ([]fly.Region, *fly.Region, error) {
				return []fly.Region{
					{Code: "iad", Name: "Ashburn, Virginia (US)"},
					{Code: "lax", Name: "Los Angeles, California (US)"},
					{Code: "fra", Name: "Frankfurt, Germany"},
				}, &fly.Region{Code: "iad", Name: "Ashburn, Virginia (US)"}, nil
			},
			GenqClientFunc: func() genq.Client {
				return &mockGenqClient{}
			},
		}
		ctx = flyutil.NewContextWithClient(ctx, mockClient)

		// Create a launch plan starting with iad region
		plan := &LaunchPlan{
			AppName:    "test-app",
			OrgSlug:    "test-org",
			RegionCode: "iad", // Start with iad
		}

		originalRegion := plan.RegionCode

		result, err := DefaultPostgres(ctx, plan, true) // mpgEnabled = true

		if err != nil {
			t.Errorf("expected no error but got: %v", err)
			return
		}

		// In non-interactive mode, it should fall back to unmanaged postgres
		// and NOT change the region (since user can't be prompted)
		if result.FlyPostgres == nil {
			t.Errorf("expected fly postgres plan but got nil")
		}
		if result.ManagedPostgres != nil {
			t.Errorf("expected no managed postgres plan but got one")
		}

		// Region should remain unchanged in non-interactive mode
		if plan.RegionCode != originalRegion {
			t.Errorf("expected region to remain %s but it changed to %s", originalRegion, plan.RegionCode)
		}
	})
}

func TestCreateFlyPostgresPlan(t *testing.T) {
	plan := &LaunchPlan{
		AppName:    "test-app",
		OrgSlug:    "test-org",
		RegionCode: "iad",
	}

	result := createFlyPostgresPlan(plan)

	if result.FlyPostgres == nil {
		t.Errorf("expected FlyPostgres plan but got nil")
		return
	}

	if result.FlyPostgres.AppName != "test-app-db" {
		t.Errorf("expected app name 'test-app-db' but got '%s'", result.FlyPostgres.AppName)
	}

	if result.FlyPostgres.VmSize != "shared-cpu-1x" {
		t.Errorf("expected vm size 'shared-cpu-1x' but got '%s'", result.FlyPostgres.VmSize)
	}

	if result.FlyPostgres.VmRam != 256 {
		t.Errorf("expected vm ram 256 but got %d", result.FlyPostgres.VmRam)
	}

	if result.FlyPostgres.DiskSizeGB != 1 {
		t.Errorf("expected disk size 1 but got %d", result.FlyPostgres.DiskSizeGB)
	}

	if result.ManagedPostgres != nil {
		t.Errorf("expected no managed postgres plan but got one")
	}
}

func TestCreateManagedPostgresPlan(t *testing.T) {
	ctx := context.Background()
	ctx = iostreams.NewContext(ctx, iostreams.System())

	plan := &LaunchPlan{
		AppName:    "test-app",
		OrgSlug:    "test-org",
		RegionCode: "iad",
	}

	result := createManagedPostgresPlan(ctx, plan, "basic")

	if result.ManagedPostgres == nil {
		t.Errorf("expected ManagedPostgres plan but got nil")
		return
	}

	if result.ManagedPostgres.DbName != "test-app-db" {
		t.Errorf("expected db name 'test-app-db' but got '%s'", result.ManagedPostgres.DbName)
	}

	if result.ManagedPostgres.Region != "iad" {
		t.Errorf("expected region 'iad' but got '%s'", result.ManagedPostgres.Region)
	}

	if result.ManagedPostgres.Plan != "basic" {
		t.Errorf("expected plan 'basic' but got '%s'", result.ManagedPostgres.Plan)
	}

	if result.ManagedPostgres.DiskSize != 10 {
		t.Errorf("expected disk size 10 but got %d", result.ManagedPostgres.DiskSize)
	}

	if result.FlyPostgres != nil {
		t.Errorf("expected no fly postgres plan but got one")
	}
}
