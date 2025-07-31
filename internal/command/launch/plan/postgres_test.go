package plan

import (
	"context"
	"testing"

	"github.com/spf13/pflag"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/iostreams"
)

func TestDefaultPostgres_ForceTypes(t *testing.T) {
	tests := []struct {
		name         string
		dbFlag       string
		mpgEnabled   bool
		expectedType string // "managed", "unmanaged", or "default"
		expectError  bool
	}{
		{
			name:         "force managed postgres",
			dbFlag:       "mpg",
			mpgEnabled:   true,
			expectedType: "managed",
			expectError:  false,
		},
		{
			name:         "force unmanaged postgres",
			dbFlag:       "upg",
			mpgEnabled:   true,
			expectedType: "unmanaged",
			expectError:  false,
		},
		{
			name:         "force legacy postgres",
			dbFlag:       "legacy",
			mpgEnabled:   true,
			expectedType: "unmanaged",
			expectError:  false,
		},
		{
			name:         "default behavior with mpg enabled",
			dbFlag:       "true",
			mpgEnabled:   true,
			expectedType: "default",
			expectError:  false,
		},
		{
			name:         "default behavior with mpg disabled",
			dbFlag:       "true",
			mpgEnabled:   false,
			expectedType: "unmanaged",
			expectError:  false,
		},
		{
			name:         "force unmanaged overrides mpg enabled",
			dbFlag:       "upg",
			mpgEnabled:   true,
			expectedType: "unmanaged",
			expectError:  false,
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

			// Create a mock launch plan
			plan := &LaunchPlan{
				AppName:    "test-app",
				OrgSlug:    "test-org",
				RegionCode: "iad", // Use a region that might not support MPG
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
			case "default":
				// For default behavior, we'll get unmanaged since iad likely doesn't support MPG in test
				if result.FlyPostgres == nil && result.ManagedPostgres == nil {
					t.Errorf("expected some postgres plan but got neither")
				}
			}
		})
	}
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
