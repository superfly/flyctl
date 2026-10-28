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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
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
type fakeGenqClient struct {
	data string
	err  error
}

func (c fakeGenqClient) MakeRequest(_ context.Context, _ *genq.Request, resp *genq.Response) error {
	if c.err != nil {
		return c.err
	}

	return json.Unmarshal([]byte(c.data), resp.Data)
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

						return fakeGenqClient{err: context.Canceled}
					}

					return fakeGenqClient{
						data: `{"createLimitedAccessToken":{"limitedAccessToken":{"tokenHeader":"` + token + `"}}}`,
						err:  tc.genqErr,
					}
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
