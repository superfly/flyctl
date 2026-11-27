package launch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	genq "github.com/Khan/genqlient/graphql"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/internal/uiex"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/scanner"
)

// A database or storage provider that failed used to be printed and
// dropped, so launch deployed the app without it and exited 0. Launch must
// stop before the deploy, with fly.toml written so `fly deploy` works once
// the failure is fixed.
func TestLaunchStopsBeforeDeployWhenProvisioningFails(t *testing.T) {
	launch := func(t *testing.T, args ...string) (out, errOut *bytes.Buffer, dir string, err error) {
		t.Helper()
		regionsErr := errors.New("regions unavailable")

		ios, _, out, errOut := iostreams.Test()
		ctx := iostreams.NewContext(context.Background(), ios)
		flags := New().Flags()
		require.NoError(t, flags.Parse(args))
		ctx = flagctx.NewContext(ctx, flags)
		ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{})
		ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
			CreateAppFunc: func(_ context.Context, req flaps.CreateAppRequest) (*flaps.App, error) {
				return &flaps.App{Name: req.Name}, nil
			},
			WaitForAppFunc: func(context.Context, string) error { return nil },
			// Nothing before createDatabases needs regions; Upstash does.
			GetRegionsFunc: func(context.Context) (*flaps.RegionData, error) { return nil, regionsErr },
		})

		dir = t.TempDir()
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

		return out, errOut, dir, state.Launch(ctx)
	}

	t.Run("stops with recovery steps", func(t *testing.T) {
		out, errOut, dir, err := launch(t)

		assert.ErrorContains(t, err, "regions unavailable")
		assert.ErrorContains(t, err, "app my-app was created")
		assert.ErrorContains(t, err, "fly deploy")
		// fly deploy never provisions anything, so the error must say how to
		// provision what failed before deploying.
		assert.ErrorContains(t, err, "fly redis create")
		// fly redis create doesn't attach; launch set REDIS_URL itself.
		assert.ErrorContains(t, err, "REDIS_URL")
		// Plan steps stop before fly.toml exists, so name the app.
		assert.ErrorContains(t, err, "-a my-app")
		// A launch with --path writes fly.toml elsewhere; deploy must find it.
		assert.ErrorContains(t, err, deployHint(dir, filepath.Join(dir, "fly.toml")))
		// Redis may exist already; don't send them to create a second one.
		assert.ErrorContains(t, err, "fly redis list")
		assert.Contains(t, errOut.String(), "Error provisioning Upstash Redis: regions unavailable")
		assert.NotContains(t, out.String(), "Your app is ready")
		assert.FileExists(t, filepath.Join(dir, "fly.toml"))
	})

	t.Run("a later failure keeps the provisioning error", func(t *testing.T) {
		_, _, _, err := launch(t, "--secret", "no-equals-sign")

		assert.ErrorContains(t, err, "invalid secret format")
		assert.ErrorContains(t, err, "regions unavailable")
		assert.ErrorContains(t, err, "fly redis create")
	})
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

// The plan's memory is written as `memory` only: a memory_mb beside it takes
// precedence, so editing `memory` in fly.toml later would have no effect.
func TestUpdateConfigWritesPlanMemoryOnce(t *testing.T) {
	newState := func(compute *appconfig.Compute, memoryMB int) *launchState {
		return &launchState{
			LaunchManifest: LaunchManifest{Plan: &plan.LaunchPlan{
				AppName:  "my-app",
				MemoryMB: memoryMB,
				Compute:  []*appconfig.Compute{compute},
			}},
			planBuildCache: planBuildCache{appConfig: appconfig.NewConfig()},
		}
	}

	t.Run("memory set from a guest", func(t *testing.T) {
		compute := guestToCompute(&fly.MachineGuest{CPUKind: "shared", CPUs: 1, MemoryMB: 1024, KernelArgs: []string{"x"}})
		state := newState(compute, 1024)

		state.updateConfig(context.Background())

		got := state.appConfig.Compute[0]
		assert.Equal(t, "1gb", got.Memory)
		assert.Zero(t, got.MemoryMB)
	})

	t.Run("plan memory without a memory key", func(t *testing.T) {
		compute := &appconfig.Compute{MachineGuest: &fly.MachineGuest{CPUKind: "shared", CPUs: 1}}
		state := newState(compute, 512)

		state.updateConfig(context.Background())

		got := state.appConfig.Compute[0]
		assert.Equal(t, "512mb", got.Memory)
		assert.Zero(t, got.MemoryMB)
	})

	t.Run("an explicit memory_mb is kept", func(t *testing.T) {
		compute := &appconfig.Compute{MachineGuest: &fly.MachineGuest{CPUKind: "shared", CPUs: 1, MemoryMB: 2048}}
		state := newState(compute, 1024)

		state.updateConfig(context.Background())

		got := state.appConfig.Compute[0]
		assert.Equal(t, 2048, got.MemoryMB)
		assert.Empty(t, got.Memory)
	})
}

// The plan's memory comes from the default process group, so it must not
// replace a `memory` another group already has.
func TestUpdateConfigKeepsExistingMemory(t *testing.T) {
	app := guestToCompute(&fly.MachineGuest{CPUKind: "shared", CPUs: 1, MemoryMB: 1024, KernelArgs: []string{"x"}})
	worker := &appconfig.Compute{
		Memory:       "4gb",
		MachineGuest: &fly.MachineGuest{CPUKind: "shared", CPUs: 2},
		Processes:    []string{"worker"},
	}
	verbatim := &appconfig.Compute{
		Memory:       "1024mb",
		MachineGuest: &fly.MachineGuest{CPUKind: "shared", CPUs: 1},
		Processes:    []string{"cron"},
	}
	state := &launchState{
		LaunchManifest: LaunchManifest{Plan: &plan.LaunchPlan{
			AppName:  "my-app",
			MemoryMB: 1024,
			Compute:  []*appconfig.Compute{app, worker, verbatim},
		}},
		planBuildCache: planBuildCache{appConfig: appconfig.NewConfig()},
	}

	state.updateConfig(context.Background())

	got := state.appConfig.Compute
	assert.Equal(t, "1gb", got[0].Memory)
	assert.Equal(t, "4gb", got[1].Memory)
	assert.Equal(t, "1024mb", got[2].Memory)
	for _, c := range got {
		assert.Zero(t, c.MemoryMB)
	}
}

// A vm_memory edited in a manifest (the Launch UI does this, leaving the
// plan's compute as it was) replaces the default group's memory and nothing
// else.
func TestUpdateConfigPlanMemoryOverridesDefaultCompute(t *testing.T) {
	app := guestToCompute(&fly.MachineGuest{CPUKind: "shared", CPUs: 1, MemoryMB: 1024, KernelArgs: []string{"x"}})
	worker := &appconfig.Compute{
		Memory:       "4gb",
		MachineGuest: &fly.MachineGuest{CPUKind: "shared", CPUs: 2},
		Processes:    []string{"worker"},
	}
	state := &launchState{
		LaunchManifest: LaunchManifest{Plan: &plan.LaunchPlan{
			AppName:  "my-app",
			MemoryMB: 2048,
			Compute:  []*appconfig.Compute{app, worker},
		}},
		planBuildCache: planBuildCache{appConfig: appconfig.NewConfig()},
	}

	state.updateConfig(context.Background())

	got := state.appConfig.Compute
	assert.Equal(t, "2gb", got[0].Memory)
	assert.Zero(t, got[0].MemoryMB)
	assert.Equal(t, "4gb", got[1].Memory)
}

// With named process groups, the default group is the first by name, and its
// compute is the one the plan's memory belongs to.
func TestUpdateConfigPlanMemoryWithNamedGroups(t *testing.T) {
	cfg := appconfig.NewConfig()
	cfg.Processes = map[string]string{"web": "serve", "worker": "work"}
	web := &appconfig.Compute{
		Memory:       "1gb",
		MachineGuest: &fly.MachineGuest{CPUKind: "shared", CPUs: 1},
		Processes:    []string{"web"},
	}
	worker := &appconfig.Compute{
		Memory:       "4gb",
		MachineGuest: &fly.MachineGuest{CPUKind: "shared", CPUs: 2},
		Processes:    []string{"worker"},
	}
	state := &launchState{
		LaunchManifest: LaunchManifest{Plan: &plan.LaunchPlan{
			AppName:  "my-app",
			MemoryMB: 2048,
			Compute:  []*appconfig.Compute{worker, web},
		}},
		planBuildCache: planBuildCache{appConfig: cfg},
	}

	state.updateConfig(context.Background())

	assert.Equal(t, "2gb", web.Memory)
	assert.Equal(t, "4gb", worker.Memory)
}

// A nil entry in the plan's compute is skipped, not dereferenced, and
// doesn't stop an edited vm_memory reaching the default group.
func TestUpdateConfigSkipsNilCompute(t *testing.T) {
	app := guestToCompute(&fly.MachineGuest{CPUKind: "shared", CPUs: 1, MemoryMB: 1024, KernelArgs: []string{"x"}})
	state := &launchState{
		LaunchManifest: LaunchManifest{Plan: &plan.LaunchPlan{
			AppName:  "my-app",
			MemoryMB: 2048,
			Compute:  []*appconfig.Compute{nil, app},
		}},
		planBuildCache: planBuildCache{appConfig: appconfig.NewConfig()},
	}

	assert.NotPanics(t, func() { state.updateConfig(context.Background()) })
	assert.Equal(t, "2gb", app.Memory)
}

func TestMemoryString(t *testing.T) {
	assert.Equal(t, "1gb", memoryString(1024))
	assert.Equal(t, "4gb", memoryString(4096))
	assert.Equal(t, "256mb", memoryString(256))
	assert.Equal(t, "1536mb", memoryString(1536))
}

func TestComputeExceedsSuspendLimit(t *testing.T) {
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
			name: "shared-cpu-1x (256MB)",
			compute: &appconfig.Compute{
				Size:         "shared-cpu-1x",
				MachineGuest: &fly.MachineGuest{},
			},
			expected: false,
		},
		{
			name: "shared-cpu-2x (512MB)",
			compute: &appconfig.Compute{
				Size:         "shared-cpu-2x",
				MachineGuest: &fly.MachineGuest{},
			},
			expected: false,
		},
		{
			name: "shared-cpu-4x (1024MB)",
			compute: &appconfig.Compute{
				Size:         "shared-cpu-4x",
				MachineGuest: &fly.MachineGuest{},
			},
			expected: false,
		},
		{
			name: "shared-cpu-8x (2048MB)",
			compute: &appconfig.Compute{
				Size:         "shared-cpu-8x",
				MachineGuest: &fly.MachineGuest{},
			},
			expected: true,
		},
		{
			name: "performance-1x (2048MB)",
			compute: &appconfig.Compute{
				Size:         "performance-1x",
				MachineGuest: &fly.MachineGuest{},
			},
			expected: true,
		},
		{
			name: "performance-2x (4096MB)",
			compute: &appconfig.Compute{
				Size:         "performance-2x",
				MachineGuest: &fly.MachineGuest{},
			},
			expected: true,
		},
		{
			name: "performance-4x (8192MB)",
			compute: &appconfig.Compute{
				Size:         "performance-4x",
				MachineGuest: &fly.MachineGuest{},
			},
			expected: true,
		},
		{
			name: "a100-40gb GPU preset",
			compute: &appconfig.Compute{
				Size:         "a100-40gb",
				MachineGuest: &fly.MachineGuest{},
			},
			expected: true,
		},
		{
			name: "GPUKind directly on MachineGuest",
			compute: &appconfig.Compute{
				Size: "shared-cpu-1x",
				MachineGuest: &fly.MachineGuest{
					GPUKind: "l40s",
				},
			},
			expected: true,
		},
		{
			name: "Memory string exceeds 2GB (4gb)",
			compute: &appconfig.Compute{
				Size:   "shared-cpu-1x",
				Memory: "4gb",
			},
			expected: true,
		},
		{
			name: "Memory string overrides size to under 2GB (1024mb)",
			compute: &appconfig.Compute{
				Size:   "performance-2x",
				Memory: "1024mb",
			},
			expected: false,
		},
		{
			name: "MemoryMB on MachineGuest exceeds 2GB",
			compute: &appconfig.Compute{
				MachineGuest: &fly.MachineGuest{
					MemoryMB: 4096,
				},
			},
			expected: true,
		},
		{
			name: "Size set without MachineGuest (nil MachineGuest)",
			compute: &appconfig.Compute{
				Size: "performance-2x",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computeExceedsSuspendLimit(tt.compute)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUpdateConfigAutoStopSuspendFallback(t *testing.T) {
	newCtx := func(t *testing.T, autoStop string) context.Context {
		t.Helper()
		flags := New().Flags()
		require.NoError(t, flags.Parse([]string{"--auto-stop", autoStop}))
		return flagctx.NewContext(context.Background(), flags)
	}

	t.Run("size performance-2x falls back to stop", func(t *testing.T) {
		ctx := newCtx(t, "suspend")
		state := &launchState{
			LaunchManifest: LaunchManifest{
				Plan: &plan.LaunchPlan{
					HttpServicePort: 8080,
					Compute: []*appconfig.Compute{
						{
							Size:         "performance-2x",
							MachineGuest: &fly.MachineGuest{},
						},
					},
				},
				Config: appconfig.NewConfig(),
			},
		}

		state.updateConfig(ctx)

		require.NotNil(t, state.appConfig.HTTPService)
		require.NotNil(t, state.appConfig.HTTPService.AutoStopMachines)
		assert.Equal(t, fly.MachineAutostopStop, *state.appConfig.HTTPService.AutoStopMachines)
	})

	t.Run("size shared-cpu-1x keeps suspend", func(t *testing.T) {
		ctx := newCtx(t, "suspend")
		state := &launchState{
			LaunchManifest: LaunchManifest{
				Plan: &plan.LaunchPlan{
					HttpServicePort: 8080,
					Compute: []*appconfig.Compute{
						{
							Size:         "shared-cpu-1x",
							MachineGuest: &fly.MachineGuest{},
						},
					},
				},
				Config: appconfig.NewConfig(),
			},
		}

		state.updateConfig(ctx)

		require.NotNil(t, state.appConfig.HTTPService)
		require.NotNil(t, state.appConfig.HTTPService.AutoStopMachines)
		assert.Equal(t, fly.MachineAutostopSuspend, *state.appConfig.HTTPService.AutoStopMachines)
	})

	t.Run("plan VmSize performance-4x falls back to stop", func(t *testing.T) {
		ctx := newCtx(t, "suspend")
		state := &launchState{
			LaunchManifest: LaunchManifest{
				Plan: &plan.LaunchPlan{
					HttpServicePort: 8080,
					VmSize:          "performance-4x",
				},
				Config: appconfig.NewConfig(),
			},
		}

		state.updateConfig(ctx)

		require.NotNil(t, state.appConfig.HTTPService)
		require.NotNil(t, state.appConfig.HTTPService.AutoStopMachines)
		assert.Equal(t, fly.MachineAutostopStop, *state.appConfig.HTTPService.AutoStopMachines)
	})

	t.Run("plan MemoryMB 4096 falls back to stop", func(t *testing.T) {
		ctx := newCtx(t, "suspend")
		state := &launchState{
			LaunchManifest: LaunchManifest{
				Plan: &plan.LaunchPlan{
					HttpServicePort: 8080,
					MemoryMB:        4096,
				},
				Config: appconfig.NewConfig(),
			},
		}

		state.updateConfig(ctx)

		require.NotNil(t, state.appConfig.HTTPService)
		require.NotNil(t, state.appConfig.HTTPService.AutoStopMachines)
		assert.Equal(t, fly.MachineAutostopStop, *state.appConfig.HTTPService.AutoStopMachines)
	})
}

func TestDeployHintKeepsPathsOutOfTheCommand(t *testing.T) {
	// PowerShell expands $app inside double quotes and cmd.exe %app%, so a
	// path can't be pasted into a command safely for every shell.
	dir := filepath.Join(t.TempDir(), "my $app")

	assert.Equal(t, "`fly deploy` in "+dir, deployHint(dir, filepath.Join(dir, "fly.toml")))
	assert.Equal(t, "`fly deploy` in "+dir+" with -c staging.toml", deployHint(dir, filepath.Join(dir, "staging.toml")))
	// The source directory is the build context, wherever the config lives.
	assert.Equal(t, "`fly deploy` in "+dir+" with -c "+filepath.Join("deploy", "staging.toml"), deployHint(dir, filepath.Join(dir, "deploy", "staging.toml")))
}

// A cluster that exists but failed to attach may already have the app's
// database and default user: retrying needs --yes and a new user.
func TestPostgresAttachRetry(t *testing.T) {
	retry := postgresAttachRetry("my-app-db", "my-app")

	assert.Contains(t, retry, "fly postgres attach --app my-app my-app-db")
	assert.Contains(t, retry, "--yes")
	assert.Regexp(t, `--database-user my-app-db-\d+`, retry)

	err := attachFailed(errors.New("secrets unavailable"), retry)
	assert.ErrorIs(t, err, errPostgresAttach)
	assert.ErrorContains(t, err, retry)
}

// Launching onto an existing Managed Postgres cluster sets the URL `fly mpg
// attach` sets, on the plan's database. The v1 API has no credentials for a
// v2 cluster, so reading them there set DATABASE_URL to just "/my-app-db".
func TestAttachExistingManagedPostgres(t *testing.T) {
	attach := func(t *testing.T, flapsClient *mock.FlapsClient, v1 *mock.MpgV1Client, v2 *mock.MpgV2Client) (map[string]*string, error) {
		t.Helper()
		var secrets map[string]*string
		flapsClient.GetAppFunc = func(context.Context, string) (*flaps.App, error) {
			return &flaps.App{Name: "my-app", Organization: flaps.AppOrganizationInfo{Slug: "my-org"}}, nil
		}
		flapsClient.UpdateAppSecretsFunc = func(_ context.Context, _ string, values map[string]*string) (*fly.UpdateAppSecretsResp, error) {
			secrets = values

			return &fly.UpdateAppSecretsResp{Version: 1}, nil
		}

		// Setting secrets saves their version to the config file, whose lock
		// lands in the working directory when no config dir is set.
		dir := t.TempDir()
		t.Chdir(dir)
		previous := viper.Get(flyctl.ConfigAppSecretsMinvers)
		t.Cleanup(func() { viper.Set(flyctl.ConfigAppSecretsMinvers, previous) })

		ios, _, _, _ := iostreams.Test()
		ctx := iostreams.NewContext(context.Background(), ios)
		ctx = state.WithConfigDirectory(ctx, dir)
		ctx = flapsutil.NewContextWithClient(ctx, flapsClient)
		ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
			GetOrganizationFunc: func(context.Context, string) (*uiex.Organization, error) {
				return &uiex.Organization{RawSlug: "my-org"}, nil
			},
		})
		ctx = mpgv1.NewContextWithClient(ctx, v1)
		ctx = mpgv2.NewContextWithClient(ctx, v2)

		launch := &launchState{LaunchManifest: LaunchManifest{Plan: &plan.LaunchPlan{
			AppName:  "my-app",
			Postgres: plan.PostgresPlan{ManagedPostgres: &plan.ManagedPostgresPlan{DbName: "my-app-db", ClusterID: "cluster-id"}},
		}}}
		err := launch.createManagedPostgres(ctx)

		return secrets, err
	}

	// v2Cluster stubs a v2 cluster whose credentials carry databaseURL and
	// the pooled pgbouncerURI. The v1 API finds the cluster but, as for any
	// v2 cluster, has no credentials.
	v2Cluster := func(databaseURL, pgbouncerURI string) (*mock.FlapsClient, *mock.MpgV1Client, *mock.MpgV2Client) {
		flapsClient := &mock.FlapsClient{
			GetManagedPostgresClusterFunc: func(_ context.Context, id string) (flaps.ManagedPostgresCluster, error) {
				return flaps.ManagedPostgresCluster{ID: id, Organization: flaps.ManagedPostgresOrganization{Slug: "my-org"}}, nil
			},
		}
		v1 := &mock.MpgV1Client{
			GetManagedClusterByIdFunc: func(_ context.Context, id string) (mpgv1.GetManagedClusterResponse, error) {
				return mpgv1.GetManagedClusterResponse{Data: mpgv1.ManagedCluster{Id: id, Version: 2, Organization: fly.Organization{Slug: "my-org"}}}, nil
			},
		}
		v2 := &mock.MpgV2Client{
			GetClusterByIdFunc: func(_ context.Context, id string) (mpgv2.GetClusterResponse, error) {
				return mpgv2.GetClusterResponse{
					Data: mpgv2.ManagedCluster{Id: id, Organization: fly.Organization{Slug: "my-org"}},
					Credentials: mpgv2.GetClusterCredentialsResponse{
						User:          "fly-user",
						Password:      "secret",
						DBName:        "fly-db",
						ConnectionUri: pgbouncerURI,
						DatabaseURL:   databaseURL,
					},
				}, nil
			},
		}

		return flapsClient, v1, v2
	}

	t.Run("v2 cluster", func(t *testing.T) {
		flapsClient, v1, v2 := v2Cluster(
			"postgresql://fly-user:secret@direct.cluster-id.flympg.net/fly-db",
			"postgresql://fly-user:secret@pgbouncer.cluster-id.flympg.net/fly-db",
		)
		secrets, err := attach(t, flapsClient, v1, v2)

		require.NoError(t, err)
		require.Contains(t, secrets, "DATABASE_URL")
		assert.Equal(t, "postgresql://fly-user:secret@direct.cluster-id.flympg.net/my-app-db", *secrets["DATABASE_URL"])
	})

	// A ui-ex without database_url sends only the pooled URI.
	t.Run("v2 cluster without database_url", func(t *testing.T) {
		flapsClient, v1, v2 := v2Cluster("", "postgresql://fly-user:secret@pgbouncer.cluster-id.flympg.net/fly-db")
		secrets, err := attach(t, flapsClient, v1, v2)

		require.NoError(t, err)
		require.Contains(t, secrets, "DATABASE_URL")
		assert.Equal(t, "postgresql://fly-user:secret@pgbouncer.cluster-id.flympg.net/my-app-db", *secrets["DATABASE_URL"])
	})

	t.Run("v2 cluster without credentials yet", func(t *testing.T) {
		flapsClient, v1, v2 := v2Cluster("", "")
		secrets, err := attach(t, flapsClient, v1, v2)

		assert.ErrorContains(t, err, "connection URI is empty")
		assert.Nil(t, secrets)
	})

	t.Run("v1 cluster", func(t *testing.T) {
		flapsClient := &mock.FlapsClient{
			GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
				return flaps.ManagedPostgresCluster{}, flaps.ErrFlapsNotFound
			},
		}
		v1 := &mock.MpgV1Client{
			GetManagedClusterByIdFunc: func(_ context.Context, id string) (mpgv1.GetManagedClusterResponse, error) {
				return mpgv1.GetManagedClusterResponse{
					Data:        mpgv1.ManagedCluster{Id: id, Version: 1, Organization: fly.Organization{Slug: "my-org"}},
					Credentials: mpgv1.GetManagedClusterCredentialsResponse{ConnectionUri: "postgresql://fly-user:secret@pgbouncer.v1.flympg.net/fly-db"},
				}, nil
			},
		}
		v2 := &mock.MpgV2Client{
			GetClusterByIdFunc: func(context.Context, string) (mpgv2.GetClusterResponse, error) {
				return mpgv2.GetClusterResponse{}, errors.New("v2 API asked about a v1 cluster")
			},
		}
		secrets, err := attach(t, flapsClient, v1, v2)

		require.NoError(t, err)
		require.Contains(t, secrets, "DATABASE_URL")
		assert.Equal(t, "postgresql://fly-user:secret@pgbouncer.v1.flympg.net/my-app-db", *secrets["DATABASE_URL"])
	})
}

// Phoenix and others generate secrets, set on the app before fly.toml is
// written. --no-create-app has no app to set them on: say which to set
// later instead of failing the config-only launch.
func TestScannerCreateSecretsWithoutApp(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), ios)
	flags := New().Flags()
	require.NoError(t, flags.Parse([]string{"--no-create-app"}))
	ctx = flagctx.NewContext(ctx, flags)
	// No stubs: any secrets call on the missing app panics.
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{})

	state := &launchState{
		LaunchManifest: LaunchManifest{Plan: &plan.LaunchPlan{AppName: "my-app"}},
		planBuildCache: planBuildCache{sourceInfo: &scanner.SourceInfo{Secrets: []scanner.Secret{{
			Key:      "SECRET_KEY_BASE",
			Generate: func() (string, error) { return "generated", nil },
		}}}},
	}

	require.NoError(t, state.scannerCreateSecrets(ctx))
	assert.Contains(t, out.String(), "SECRET_KEY_BASE")
	assert.Contains(t, out.String(), "fly secrets set")
	assert.NotContains(t, out.String(), "generated")
}

// fakeGenqClient answers every GraphQL request with data, or fails with err.
func fakeGenqClient(data string, err error) genq.Client {
	return mock.GraphQLClient(func(_ context.Context, _ *genq.Request, resp *genq.Response) error {
		if err != nil {
			return err
		}

		return json.Unmarshal([]byte(data), resp.Data)
	})
}

// The deploy token for GitHub Actions is made after the app exists. A token
// that can't create tokens (a deploy token, say) used to fail the launch at
// that point; now any failure says how to set the secret by hand, with the
// right token, and launch carries on. Only cancellation still stops it.
func TestSetupGitHubActionsToken(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake gh is a shell script")
	}

	const token = "FlyV1 fm2_deploy"
	errNotAuthorized := errors.New("input:3:2: createLimitedAccessToken Not authorized to access this createlimitedaccesstoken")

	for _, tc := range []struct {
		name       string
		genqErr    error
		ghExit     int
		envToken   bool
		planStep   string
		cancel     bool
		wantErr    bool
		wantSecret bool
		wantOut    []string
		wantErrOut []string
		notErrOut  []string
	}{
		{
			name:       "token set",
			wantSecret: true,
			wantOut:    []string{"Setting FLY_API_TOKEN secret"},
		},
		{
			name:       "token not allowed to create tokens",
			genqErr:    errNotAuthorized,
			wantErrOut: []string{"Couldn't create a deploy token for GitHub Actions: the token you're using isn't allowed to create tokens", "Logged in as yourself (`fly auth login`)", "fly tokens create deploy -x 999999h"},
		},
		{
			name:       "token from the environment",
			genqErr:    errNotAuthorized,
			envToken:   true,
			wantErrOut: []string{"FLY_API_TOKEN is set; without it, logged in as yourself", "fly tokens create deploy -x 999999h"},
		},
		{
			name:       "other failure",
			genqErr:    errors.New("503 Service Unavailable"),
			wantErrOut: []string{"Couldn't create a deploy token for GitHub Actions: 503 Service Unavailable", "fly tokens create deploy -x 999999h"},
			notErrOut:  []string{"fly auth login"},
		},
		{
			name:       "gh fails",
			ghExit:     1,
			wantSecret: true,
			wantErrOut: []string{"gh: not logged in", "Couldn't set the FLY_API_TOKEN secret", "fly tokens create deploy -x 999999h"},
		},
		{
			name:       "plan step warns without the steps",
			genqErr:    errNotAuthorized,
			planStep:   "generate",
			wantErrOut: []string{"Couldn't create a deploy token for GitHub Actions"},
			notErrOut:  []string{"fly tokens create deploy"},
		},
		{name: "canceled", cancel: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := t.TempDir()
			secret := filepath.Join(bin, "secret")
			gh := fmt.Sprintf("#!/bin/sh\ncat > %q\n[ %d -eq 0 ] || { echo 'gh: not logged in' >&2; exit %d; }\n", secret, tc.ghExit, tc.ghExit)
			require.NoError(t, os.WriteFile(filepath.Join(bin, "gh"), []byte(gh), 0o755))
			t.Setenv("PATH", bin+":/bin:/usr/bin")
			t.Setenv("FLY_ACCESS_TOKEN", "")
			t.Setenv("FLY_API_TOKEN", "")
			if tc.envToken {
				t.Setenv("FLY_API_TOKEN", token)
			}

			ios, _, out, errOut := iostreams.Test()
			ctx, cancel := context.WithCancel(iostreams.NewContext(context.Background(), ios))
			defer cancel()
			if tc.planStep != "" {
				ctx = context.WithValue(ctx, plan.PlanStepKey, tc.planStep)
			}
			ctx = flagctx.NewContext(ctx, New().Flags())
			ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
				GetOrganizationFunc: func(_ context.Context, slug string) (*uiex.Organization, error) {
					return &uiex.Organization{ID: "org-1", Slug: slug}, nil
				},
			})
			ctx = flyutil.NewContextWithClient(ctx, &mock.Client{
				GenqClientFunc: func() genq.Client {
					if tc.cancel {
						cancel()

						return fakeGenqClient("", context.Canceled)
					}

					return fakeGenqClient(`{"createLimitedAccessToken":{"limitedAccessToken":{"tokenHeader":"`+token+`"}}}`, tc.genqErr)
				},
			})

			state := &launchState{
				LaunchManifest: LaunchManifest{Plan: &plan.LaunchPlan{OrgSlug: "personal"}},
				planBuildCache: planBuildCache{sourceInfo: &scanner.SourceInfo{
					GitHubActions: scanner.GitHubActionsStruct{Deploy: true, Secrets: true},
				}},
				cache: map[string]any{},
			}

			err := state.setupGitHubActions(ctx, "my-app")

			if tc.wantErr {
				require.ErrorIs(t, err, context.Canceled)

				return
			}
			require.NoError(t, err)

			got, readErr := os.ReadFile(secret)
			if tc.wantSecret {
				require.NoError(t, readErr)
				assert.Equal(t, token, string(got), "gh gets the token on stdin")
			} else {
				assert.True(t, os.IsNotExist(readErr), "gh not run")
			}
			for _, want := range tc.wantOut {
				assert.Contains(t, out.String(), want)
			}
			for _, want := range tc.wantErrOut {
				assert.Contains(t, errOut.String(), want)
			}
			for _, unwanted := range tc.notErrOut {
				assert.NotContains(t, errOut.String(), unwanted)
			}
		})
	}
}

// FLYCTL-12T0: launching from a manifest leaves sourceInfo nil, and building
// the Tigris request dereferenced it, crashing `fly launch` with a nil
// pointer panic instead of creating the bucket.
func TestTigrisExtensionParamsWithoutSourceInfo(t *testing.T) {
	state := &launchState{
		LaunchManifest: LaunchManifest{Plan: &plan.LaunchPlan{
			AppName:    "my-app",
			RegionCode: "iad",
			ObjectStorage: plan.ObjectStoragePlan{
				TigrisObjectStorage: &plan.TigrisObjectStoragePlan{Name: "my-bucket"},
			},
		}},
	}

	params := state.tigrisExtensionParams(nil)
	assert.Equal(t, "tigris", params.Provider)
	assert.Equal(t, "my-bucket", *params.OverrideName)
	assert.Nil(t, params.OverrideExtensionSecretKeyNames)

	// Scanners like Laravel rename the Tigris secrets; keep passing that through.
	overrides := map[string]map[string]string{"tigris": {"BUCKET_NAME": "AWS_BUCKET"}}
	state.sourceInfo = &scanner.SourceInfo{OverrideExtensionSecretKeyNames: overrides}
	assert.Equal(t, overrides, state.tigrisExtensionParams(nil).OverrideExtensionSecretKeyNames)
}
