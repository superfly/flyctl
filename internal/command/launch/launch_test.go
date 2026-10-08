package launch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/scanner"
)

// A database or storage provider that failed used to be printed and
// dropped, so launch deployed the app without it and exited 0. Launch must
// stop before the deploy, with fly.toml written so `fly deploy` works once
// the failure is fixed.
func TestLaunchStopsBeforeDeployWhenProvisioningFails(t *testing.T) {
	regionsErr := errors.New("regions unavailable")

	ios, _, out, errOut := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), ios)
	ctx = flagctx.NewContext(ctx, New().Flags())
	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{})
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		CreateAppFunc: func(_ context.Context, req flaps.CreateAppRequest) (*flaps.App, error) {
			return &flaps.App{Name: req.Name}, nil
		},
		WaitForAppFunc: func(context.Context, string) error { return nil },
		// Nothing before createDatabases needs regions; Upstash does.
		GetRegionsFunc: func(context.Context) (*flaps.RegionData, error) { return nil, regionsErr },
	})

	dir := t.TempDir()
	state := &launchState{
		workingDir: dir,
		configPath: filepath.Join(dir, "fly.toml"),
		LaunchManifest: LaunchManifest{Plan: &plan.LaunchPlan{
			AppName: "my-app",
			Redis:   plan.RedisPlan{UpstashRedis: &plan.UpstashRedisPlan{}},
		}},
		planBuildCache: planBuildCache{
			appConfig: appconfig.NewConfig(),
			// With SkipDeploy, reaching the deploy step prints
			// "Your app is ready!" instead of deploying.
			sourceInfo: &scanner.SourceInfo{SkipDeploy: true},
		},
		cache: map[string]any{},
	}

	err := state.Launch(ctx)

	require.ErrorIs(t, err, regionsErr)
	assert.ErrorContains(t, err, "app my-app was created")
	assert.ErrorContains(t, err, "fly deploy")
	// fly deploy never provisions anything, so the error must say how to
	// provision what failed before deploying.
	assert.ErrorContains(t, err, "fly redis create")
	// fly redis create doesn't attach; launch set REDIS_URL itself.
	assert.ErrorContains(t, err, "REDIS_URL")
	assert.Contains(t, errOut.String(), "Error provisioning Upstash Redis: regions unavailable")
	assert.NotContains(t, out.String(), "Your app is ready")
	assert.FileExists(t, filepath.Join(dir, "fly.toml"))
}

// Relaunching over a file a scanner generates (Dockerfile, fly-deploy.yml)
// asks before overwriting it. Without a terminal the file is kept, and the
// output has to say so, or an agent never learns why its change is missing.
func TestScannerCreateFilesKeepsExistingFileHeadless(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Dockerfile")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))

	ios, _, out, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), ios)
	ctx = flagctx.NewContext(ctx, New().Flags())

	state := &launchState{
		workingDir: dir,
		planBuildCache: planBuildCache{sourceInfo: &scanner.SourceInfo{
			Files: []scanner.SourceFile{{Path: "Dockerfile", Contents: []byte("new")}},
		}},
	}

	require.NoError(t, state.scannerCreateFiles(ctx))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "old", string(got))
	assert.Contains(t, out.String(), "Not overwriting "+path)
	assert.Contains(t, out.String(), "--yes")
}

func TestWillCreateManagedPostgresCluster(t *testing.T) {
	tests := []struct {
		name     string
		postgres plan.PostgresPlan
		planStep string
		expected bool
	}{
		{
			name: "new managed postgres",
			postgres: plan.PostgresPlan{
				ManagedPostgres: &plan.ManagedPostgresPlan{},
			},
			expected: true,
		},
		{
			name: "postgres plan step",
			postgres: plan.PostgresPlan{
				ManagedPostgres: &plan.ManagedPostgresPlan{},
			},
			planStep: "postgres",
			expected: true,
		},
		{
			name: "deploy plan step",
			postgres: plan.PostgresPlan{
				ManagedPostgres: &plan.ManagedPostgresPlan{},
			},
			planStep: "deploy",
			expected: false,
		},
		{
			name: "existing managed postgres",
			postgres: plan.PostgresPlan{
				ManagedPostgres: &plan.ManagedPostgresPlan{ClusterID: "mpg_123"},
			},
			expected: false,
		},
		{
			name: "unmanaged postgres",
			postgres: plan.PostgresPlan{
				FlyPostgres: &plan.FlyPostgresPlan{},
			},
			expected: false,
		},
		{
			name:     "no postgres",
			postgres: plan.PostgresPlan{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &launchState{
				LaunchManifest: LaunchManifest{
					Plan: &plan.LaunchPlan{Postgres: tt.postgres},
				},
			}

			assert.Equal(t, tt.expected, state.willCreateManagedPostgresCluster(tt.planStep))
		})
	}
}

func TestManagedPostgresCreationPrompt(t *testing.T) {
	state := &launchState{
		LaunchManifest: LaunchManifest{
			Plan: &plan.LaunchPlan{
				AppName:    "my-app",
				RegionCode: "iad",
				Postgres: plan.PostgresPlan{
					ManagedPostgres: &plan.ManagedPostgresPlan{
						Plan: "basic",
					},
				},
			},
		},
	}

	assert.Equal(t,
		`This launch will create a new Managed Postgres database, "my-app-db", on the Basic plan ($38/mo) in iad. This is an additional paid resource. How would you like to proceed?`,
		state.managedPostgresCreationPrompt(),
	)
}

func TestLaunchWithoutManagedPostgresCluster(t *testing.T) {
	state := &launchState{
		LaunchManifest: LaunchManifest{
			Plan: &plan.LaunchPlan{
				Postgres: plan.PostgresPlan{
					ManagedPostgres: &plan.ManagedPostgresPlan{
						Plan: "basic",
					},
				},
			},
		},
	}

	state.launchWithoutManagedPostgresCluster()

	assert.Nil(t, state.Plan.Postgres.ManagedPostgres)
	assert.False(t, state.willCreateManagedPostgresCluster(""))
}

func TestIsComputeValid(t *testing.T) {
	tests := []struct {
		name     string
		compute  *appconfig.Compute
		expected bool
	}{
		{
			name:     "nil compute",
			compute:  nil,
			expected: false,
		},
		{
			name: "compute with nil MachineGuest",
			compute: &appconfig.Compute{
				MachineGuest: nil,
			},
			expected: false,
		},
		{
			name: "valid compute with MachineGuest",
			compute: &appconfig.Compute{
				MachineGuest: &fly.MachineGuest{
					CPUKind:  "shared",
					CPUs:     1,
					MemoryMB: 256,
				},
			},
			expected: true,
		},
		{
			name: "valid compute with empty MachineGuest",
			compute: &appconfig.Compute{
				MachineGuest: &fly.MachineGuest{},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isComputeValid(tt.compute)
			assert.Equal(t, tt.expected, result)
		})
	}
}
