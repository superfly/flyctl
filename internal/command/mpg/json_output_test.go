package mpg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiex"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

// Exercise dispatch as well as the API runners. The legacy-v2 cases require a
// public 404 before the old client is called; v1 selects the separate runner.
func TestMutationOutput(t *testing.T) {
	for _, operation := range []string{"create", "fork", "restore", "users create", "databases create"} {
		backends := []string{"public"}
		if operation != "create" && operation != "fork" {
			backends = append(backends, "legacy-v2", "v1")
		}
		for _, backend := range backends {
			for _, mode := range []string{"json", "text", "failure", "write-failure"} {
				t.Run(operation+"/"+backend+"/"+mode, func(t *testing.T) {
					ios, _, stdout, stderr := iostreams.Test()
					jsonOutput := mode != "text"
					// Even with a terminal, JSON mode must not need any prompts.
					ios.SetStdinTTY(jsonOutput)
					ios.SetStdoutTTY(jsonOutput)
					writeErr := errors.New("broken stdout")
					if mode == "write-failure" {
						ios.Out = mutationErrorWriter{writeErr}
					}
					ctx := iostreams.NewContext(context.Background(), ios)
					ctx = config.NewContext(ctx, &config.Config{JSONOutput: jsonOutput, Organization: "my-org"})
					cmd, args, run := mutationCommand(operation)
					if jsonOutput {
						args = append(args, "--json")
					}
					require.NoError(t, cmd.Flags().Parse(args))
					ctx = flagctx.NewContext(ctx, cmd.Flags())

					var apiErr error
					if mode == "failure" {
						apiErr = errors.New("request failed")
					}
					cluster := flaps.ManagedPostgresCluster{
						ID: "new-id", Name: "my-db", Region: "iad", Plan: "basic", Status: "creating",
						Organization: flaps.ManagedPostgresOrganization{Name: "My Org", Slug: "my-org"},
					}
					legacyCluster := mpgv2.ManagedCluster{
						Id: cluster.ID, Name: cluster.Name, Region: cluster.Region, Plan: cluster.Plan, Status: cluster.Status,
						Organization: fly.Organization{Name: "My Org", Slug: "my-org"},
					}
					v1Cluster := mpgv1.ManagedCluster{
						Id: cluster.ID, Name: cluster.Name, Region: cluster.Region, Plan: cluster.Plan, Status: cluster.Status,
						Organization: legacyCluster.Organization,
					}
					publicCalls, legacyCalls := 0, 0
					publicError := func() error {
						publicCalls++
						if backend != "public" {
							return flaps.ErrFlapsNotFound
						}
						return apiErr
					}
					legacyError := func() error {
						legacyCalls++
						return apiErr
					}
					ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
						ListOrganizationsFunc: func(context.Context, bool) ([]uiex.Organization, error) {
							return []uiex.Organization{{Slug: "my-org", RawSlug: "my-org"}}, nil
						},
					})
					ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
						GetRegionsFunc: func(context.Context) (*flaps.RegionData, error) {
							return &flaps.RegionData{Regions: []fly.Region{{Code: "iad", MPGAvailable: true}}}, nil
						},
						GetManagedPostgresClusterFunc: func(_ context.Context, id string) (flaps.ManagedPostgresCluster, error) {
							if backend == "v1" {
								return flaps.ManagedPostgresCluster{}, flaps.ErrFlapsNotFound
							}
							if id == "new-id" {
								ready := cluster
								ready.Status = "ready"
								ready.Endpoints.Primary.Pooler = flaps.ManagedPostgresEndpoint{Host: "pooler.example.test", Port: 6432}
								return ready, nil
							}

							return flaps.ManagedPostgresCluster{ID: "source", Organization: cluster.Organization}, nil
						},
						GetManagedPostgresUserCredentialsFunc: func(context.Context, string, string) (flaps.ManagedPostgresUserCredentials, error) {
							return flaps.ManagedPostgresUserCredentials{Username: "fly-user", Password: "test-password"}, nil
						},
						CreateManagedPostgresClusterFunc: func(context.Context, flaps.CreateManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
							return cluster, publicError()
						},
						ForkManagedPostgresClusterFunc: func(context.Context, string, flaps.ForkManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
							return cluster, publicError()
						},
						RestoreManagedPostgresClusterFunc: func(context.Context, string, flaps.RestoreManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
							return cluster, publicError()
						},
						CreateManagedPostgresUserFunc: func(context.Context, string, flaps.CreateManagedPostgresUserRequest) (flaps.ManagedPostgresUser, error) {
							return flaps.ManagedPostgresUser{Username: "app_user", Role: flaps.ManagedPostgresUserRoleWriter}, publicError()
						},
						CreateManagedPostgresDatabaseFunc: func(context.Context, string, flaps.CreateManagedPostgresDatabaseRequest) (flaps.ManagedPostgresDatabase, error) {
							return flaps.ManagedPostgresDatabase{Name: "app"}, publicError()
						},
					})
					ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
						RestoreClusterBackupFunc: func(context.Context, string, mpgv2.RestoreClusterBackupInput) (mpgv2.RestoreClusterBackupResponse, error) {
							return mpgv2.RestoreClusterBackupResponse{Data: legacyCluster}, legacyError()
						},
						CreateUserWithRoleFunc: func(context.Context, string, mpgv2.CreateUserWithRoleInput) (mpgv2.CreateUserWithRoleResponse, error) {
							return mpgv2.CreateUserWithRoleResponse{Data: mpgv2.User{Name: "app_user", Role: "writer"}}, legacyError()
						},
						CreateDatabaseFunc: func(context.Context, string, mpgv2.CreateDatabaseInput) error {
							return legacyError()
						},
					})
					ctx = mpgv1.NewContextWithClient(ctx, &mock.MpgV1Client{
						GetManagedClusterByIdFunc: func(context.Context, string) (mpgv1.GetManagedClusterResponse, error) {
							return mpgv1.GetManagedClusterResponse{Data: mpgv1.ManagedCluster{Id: "source"}}, nil
						},
						RestoreManagedClusterBackupFunc: func(context.Context, string, mpgv1.RestoreManagedClusterBackupInput) (mpgv1.RestoreManagedClusterBackupResponse, error) {
							return mpgv1.RestoreManagedClusterBackupResponse{Data: v1Cluster}, legacyError()
						},
						CreateUserWithRoleFunc: func(context.Context, string, mpgv1.CreateUserWithRoleInput) (mpgv1.CreateUserWithRoleResponse, error) {
							return mpgv1.CreateUserWithRoleResponse{Data: mpgv1.User{Name: "app_user", Role: "writer"}}, legacyError()
						},
						CreateDatabaseFunc: func(context.Context, string, mpgv1.CreateDatabaseInput) (mpgv1.CreateDatabaseResponse, error) {
							return mpgv1.CreateDatabaseResponse{Data: mpgv1.Database{Name: "app"}}, legacyError()
						},
					})
					err := run(ctx)
					if backend == "v1" {
						require.Zero(t, publicCalls)
					} else {
						require.Equal(t, 1, publicCalls)
					}
					require.Equal(t, backend != "public", legacyCalls == 1)
					if mode == "failure" {
						require.Error(t, err)
						require.Empty(t, stdout.String())
						return
					}
					if mode == "write-failure" {
						created := "cluster new-id"
						switch operation {
						case "users create":
							created = "user app_user in cluster source"
						case "databases create":
							created = "database app in cluster source"
						}
						require.ErrorIs(t, err, writeErr)
						require.EqualError(t, err, created+" was created, but its result could not be displayed: broken stdout")
						require.Empty(t, stdout.String())

						return
					}
					require.NoError(t, err)
					if mode == "text" {
						require.Equal(t, mutationText(operation), stdout.String())
						require.Empty(t, stderr.String())
						return
					}
					// JSONEq rejects extra documents, text, and unexpected fields.
					want := `{"id":"new-id","name":"my-db","organization":{"Name":"My Org","Slug":"my-org"},"region":"iad","plan":"basic","status":"creating"}`
					switch operation {
					case "create":
						want = `{"id":"new-id","name":"my-db","organization":{"Name":"My Org","Slug":"my-org"},"region":"iad","plan":"basic","status":"ready"}`
						require.Contains(t, stderr.String(), "Waiting for cluster my-db (new-id) to be ready...")
					case "users create":
						want = `{"name":"app_user","role":"writer"}`
					case "databases create":
						want = `{"name":"app"}`
					}
					require.JSONEq(t, want, stdout.String())
					require.NotEmpty(t, stderr.String())
					require.NotContains(t, stderr.String(), "test-password")
				})
			}
		}
	}
}

type mutationErrorWriter struct{ err error }

func (w mutationErrorWriter) Write([]byte) (int, error) { return 0, w.err }

func mutationCommand(operation string) (*cobra.Command, []string, func(context.Context) error) {
	switch operation {
	case "create":
		return newCreate(), []string{"--name", "my-db", "--region", "iad", "--plan", "basic"}, func(ctx context.Context) error {
			return cmdv2.RunCreate(ctx, "my-org", &cmdv2.CreateClusterParams{Name: "my-db", OrgSlug: "my-org", Plan: "basic", PGMajorVersion: 16}, &cmdv2.CreatePlanDisplay{Name: "Basic"})
		}
	case "fork":
		return newFork(), []string{"source"}, runFork
	case "restore":
		return newRestore(), []string{"source", "--backup-id", "backup"}, runRestore
	case "users create":
		return newUsersCreate(), []string{"source", "--username", "app_user", "--role", "writer"}, runUsersCreate
	case "databases create":
		return newDatabasesCreate(), []string{"source", "--name", "app"}, runDatabasesCreate
	default:
		panic(operation)
	}
}

// These literals preserve the complete output, including whitespace, of the
// commands before JSON support was added.
func mutationText(operation string) string {
	switch operation {
	case "create":
		return "Selected Plan: Basic\n  CPU: \n  Memory: \n  Price: $0 per month\n\n" +
			"Waiting for cluster my-db (new-id) to be ready...\n" +
			"You can view the cluster in the UI at: https://fly.io/dashboard/my-org/managed_postgres/new-id\n" +
			"You can cancel this wait with Ctrl+C - the cluster will continue provisioning in the background.\n" +
			"Once ready, you can connect to the database with: fly mpg connect new-id\n\n" +
			"\nManaged Postgres cluster created successfully!\n  ID: new-id\n  Name: my-db\n  Organization: my-org\n  Region: iad\n" +
			"  Plan: basic\n  Volume Size: 0GB\n  PostGIS: false\n  Connection string: postgres://fly-user:test-password@pooler.example.test:6432/fly-db\n"
	case "fork":
		return "Forking cluster source...\nFork initiated successfully!\n  Cluster ID: new-id\n  Cluster Name: my-db\n  Plan: basic\n"
	case "restore":
		return "Restoring cluster source from backup backup...\nRestore initiated successfully!\n  Cluster ID: new-id\n  Cluster Name: my-db\n"
	case "users create":
		return "Creating user app_user with role writer in cluster source...\nUser created successfully!\n  Name: app_user\n  Role: writer\n"
	case "databases create":
		return "Creating database app in cluster source...\nDatabase created successfully!\n  Name: app\n"
	default:
		panic(operation)
	}
}

func TestMutationJSONMissingInputs(t *testing.T) {
	cases := []struct {
		operation string
		args      []string
		missing   string
	}{
		{"create", nil, "--name"},
		{"create", []string{"--name", "db"}, "--org"},
		{"create", []string{"--name", "db"}, "--region"},
		{"fork", nil, "CLUSTER_ID"},
		{"restore", nil, "CLUSTER_ID"},
		{"restore", []string{"source"}, "--backup-id"},
		{"users create", nil, "CLUSTER_ID"},
		{"users create", []string{"source"}, "--username"},
		{"users create", []string{"source", "--username", "app_user"}, "--role"},
		{"databases create", nil, "CLUSTER_ID"},
		{"databases create", []string{"source"}, "--name"},
	}
	for _, tc := range cases {
		for _, v1 := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%s/v1=%t", tc.operation, tc.missing, v1), func(t *testing.T) {
				ios, _, stdout, _ := iostreams.Test()
				ios.SetStdinTTY(true)
				ios.SetStdoutTTY(true)
				ctx := iostreams.NewContext(context.Background(), ios)
				org := "my-org"
				if tc.missing == "--org" {
					org = ""
				}
				ctx = config.NewContext(ctx, &config.Config{JSONOutput: true, Organization: org})
				cmd, _, run := mutationCommand(tc.operation)
				if tc.operation == "create" {
					run = runCreate
				}
				require.NoError(t, cmd.Flags().Parse(append(tc.args, "--json")))
				ctx = flagctx.NewContext(ctx, cmd.Flags())
				ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
					ListOrganizationsFunc: func(context.Context, bool) ([]uiex.Organization, error) {
						return []uiex.Organization{{Slug: "my-org", RawSlug: "my-org"}, {Slug: "another-org"}}, nil
					},
				})
				ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
					GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
						if v1 {
							return flaps.ManagedPostgresCluster{}, flaps.ErrFlapsNotFound
						}
						return flaps.ManagedPostgresCluster{ID: "source"}, nil
					},
				})
				ctx = mpgv1.NewContextWithClient(ctx, &mock.MpgV1Client{
					GetManagedClusterByIdFunc: func(context.Context, string) (mpgv1.GetManagedClusterResponse, error) {
						return mpgv1.GetManagedClusterResponse{Data: mpgv1.ManagedCluster{Id: "source"}}, nil
					},
				})
				err := run(ctx)
				require.ErrorContains(t, err, tc.missing)
				if tc.missing != "--backup-id" {
					require.True(t, prompt.IsNonInteractive(err), "%v", err)
				}
				require.Empty(t, stdout.String())
			})
		}
	}
}

func TestForkJSONRejectsV1(t *testing.T) {
	ios, _, stdout, _ := iostreams.Test()
	ctx := config.NewContext(iostreams.NewContext(context.Background(), ios), &config.Config{JSONOutput: true})
	cmd := newFork()
	require.NoError(t, cmd.Flags().Parse([]string{"source", "--json"}))
	ctx = flagctx.NewContext(ctx, cmd.Flags())
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
			return flaps.ManagedPostgresCluster{}, flaps.ErrFlapsNotFound
		},
	})
	ctx = mpgv1.NewContextWithClient(ctx, &mock.MpgV1Client{
		GetManagedClusterByIdFunc: func(context.Context, string) (mpgv1.GetManagedClusterResponse, error) {
			return mpgv1.GetManagedClusterResponse{Data: mpgv1.ManagedCluster{Id: "source"}}, nil
		},
	})
	require.EqualError(t, runFork(ctx), "forking is not supported for this cluster")
	require.Empty(t, stdout.String())
}

func TestMutationJSONHelp(t *testing.T) {
	for operation, example := range map[string]string{
		"create": `  out=$(fly mpg create --name my-db --org my-org --region iad --plan basic --json) || { echo "$out" >&2; exit 1; }
  cluster_id=$(printf '%s' "$out" | jq -er '.id | select(. != "")') || exit 1`,
		"fork": `  out=$(fly mpg fork "$source_id" --json) || { echo "$out" >&2; exit 1; }
  cluster_id=$(printf '%s' "$out" | jq -er '.id | select(. != "")') || exit 1`,
		"restore": `  out=$(fly mpg restore "$source_id" --backup-id "$backup_id" --json) || { echo "$out" >&2; exit 1; }
  cluster_id=$(printf '%s' "$out" | jq -er '.id | select(. != "")') || exit 1`,
		"users create": `  out=$(fly mpg users create "$cluster_id" --username app_user --role writer --json) || { echo "$out" >&2; exit 1; }
  username=$(printf '%s' "$out" | jq -er '.name | select(. != "")') || exit 1`,
		"databases create": `  out=$(fly mpg databases create "$cluster_id" --name app --json) || { echo "$out" >&2; exit 1; }
  database=$(printf '%s' "$out" | jq -er '.name | select(. != "")') || exit 1`,
	} {
		t.Run(operation, func(t *testing.T) {
			cmd, _, _ := mutationCommand(operation)
			f := cmd.Flags().Lookup("json")
			require.NotNil(t, f)
			require.Equal(t, "bool", f.Value.Type())
			require.Equal(t, "j", f.Shorthand)
			require.Contains(t, f.Usage, "JSON")
			require.Equal(t, example, cmd.Example)
		})
	}
}

// Check that the cluster fields have exactly the spellings used by the read
// commands, including the Organization type's capitalized nested keys.
func TestMutationJSONMatchesReadFieldNames(t *testing.T) {
	for _, value := range []any{mpgv1.ManagedCluster{}, mpgv2.ManagedCluster{}} {
		body, err := json.Marshal(value)
		require.NoError(t, err)
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(body, &fields))
		for _, name := range []string{"id", "name", "organization", "region", "plan", "status"} {
			require.Contains(t, fields, name)
		}
		var org map[string]any
		require.NoError(t, json.Unmarshal(fields["organization"], &org))
		require.Contains(t, org, "Name")
		require.Contains(t, org, "Slug")
	}
}

func TestCreateJSONDefaultsAndWarning(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)
	ctx := config.NewContext(iostreams.NewContext(context.Background(), ios), &config.Config{JSONOutput: true})
	cmd := newCreate()
	require.NoError(t, cmd.Flags().Parse([]string{"--name", "my-db", "--region", "iad", "--json", "--v2"}))
	ctx = flagctx.NewContext(ctx, cmd.Flags())
	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
		ListOrganizationsFunc: func(context.Context, bool) ([]uiex.Organization, error) {
			return []uiex.Organization{{Name: "My Org", Slug: "personal", RawSlug: "my-org", Personal: true}}, nil
		},
	})
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetRegionsFunc: func(context.Context) (*flaps.RegionData, error) {
			return &flaps.RegionData{Regions: []fly.Region{{Code: "iad", MPGAvailable: true}}}, nil
		},
		CreateManagedPostgresClusterFunc: func(_ context.Context, req flaps.CreateManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
			require.Equal(t, "basic", req.Plan, "keep the non-interactive default plan")
			require.Equal(t, "my-org", req.OrgSlug)
			return flaps.ManagedPostgresCluster{ID: "new-id", Region: "iad"}, nil
		},
		GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
			ready := flaps.ManagedPostgresCluster{ID: "new-id", Status: "ready"}
			ready.Endpoints.Primary.Pooler = flaps.ManagedPostgresEndpoint{Host: "pooler.example.test", Port: 6432}
			return ready, nil
		},
		GetManagedPostgresUserCredentialsFunc: func(context.Context, string, string) (flaps.ManagedPostgresUserCredentials, error) {
			return flaps.ManagedPostgresUserCredentials{Password: "test-password"}, nil
		},
	})
	require.NoError(t, runCreate(ctx))
	require.JSONEq(t, `{"id":"new-id","name":"my-db","organization":{"Name":"","Slug":"my-org"},"region":"iad","plan":"basic","status":"ready"}`, stdout.String())
	require.Contains(t, stderr.String(), "automatically selected personal organization: My Org")
	require.Contains(t, stderr.String(), "The '--v2' flag is deprecated")
	require.Contains(t, stderr.String(), "Waiting for cluster")
}

func TestCreateJSONReadinessResults(t *testing.T) {
	for _, tc := range []struct {
		stage      string
		wantStatus string
		wantErr    string
	}{
		{"poll", "creating", "failed checking cluster status: poll failed"},
		{"last-poll", "provisioning", "failed checking cluster status: poll failed"},
		{"failed", "failed", "cluster creation failed"},
		{"error", "error", "cluster creation failed"},
		{"cancel-poll", "creating", "failed checking cluster status: context canceled"},
		{"cancel-wait", "provisioning", "context canceled"},
		{"invalid", "creating", "invalid cluster response: no cluster ID"},
		{"empty-status", "", "failed checking cluster status: poll failed"},
		{"credentials", "ready", ""},
		{"endpoint", "ready", ""},
	} {
		t.Run(tc.stage, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ios, _, stdout, stderr := iostreams.Test()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				ctx = config.NewContext(iostreams.NewContext(ctx, ios), &config.Config{JSONOutput: true})
				cmd := newCreate()
				require.NoError(t, cmd.Flags().Parse([]string{"--region", "iad", "--json"}))
				ctx = flagctx.NewContext(ctx, cmd.Flags())
				polls, credentials := 0, 0
				ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
					GetRegionsFunc: func(context.Context) (*flaps.RegionData, error) {
						return &flaps.RegionData{Regions: []fly.Region{{Code: "iad", MPGAvailable: true}}}, nil
					},
					CreateManagedPostgresClusterFunc: func(context.Context, flaps.CreateManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
						cluster := flaps.ManagedPostgresCluster{ID: "new-id", Region: "iad", Status: "creating"}
						if tc.stage == "empty-status" {
							cluster.Status = ""
						}
						return cluster, nil
					},
					GetManagedPostgresClusterFunc: func(ctx context.Context, id string) (flaps.ManagedPostgresCluster, error) {
						require.Equal(t, "new-id", id)
						polls++
						switch tc.stage {
						case "poll", "empty-status":
							return flaps.ManagedPostgresCluster{}, errors.New("poll failed")
						case "last-poll":
							if polls > 1 {
								return flaps.ManagedPostgresCluster{}, errors.New("poll failed")
							}
							return flaps.ManagedPostgresCluster{ID: id, Status: "provisioning"}, nil
						case "cancel-poll":
							cancel()
							return flaps.ManagedPostgresCluster{}, ctx.Err()
						case "cancel-wait":
							if polls > 1 {
								return flaps.ManagedPostgresCluster{}, ctx.Err()
							}
							cancel()
							return flaps.ManagedPostgresCluster{ID: id, Status: "provisioning"}, nil
						case "invalid":
							return flaps.ManagedPostgresCluster{Status: "failed"}, nil
						case "failed", "error":
							return flaps.ManagedPostgresCluster{ID: id, Status: tc.stage}, nil
						}
						ready := flaps.ManagedPostgresCluster{ID: id, Status: "ready"}
						if tc.stage != "endpoint" {
							ready.Endpoints.Primary.Pooler = flaps.ManagedPostgresEndpoint{Host: "pooler.example.test", Port: 6432}
						}

						return ready, nil
					},
					GetManagedPostgresUserCredentialsFunc: func(context.Context, string, string) (flaps.ManagedPostgresUserCredentials, error) {
						credentials++
						if tc.stage == "endpoint" {
							return flaps.ManagedPostgresUserCredentials{Password: "test-password"}, nil
						}
						return flaps.ManagedPostgresUserCredentials{}, errors.New("credentials failed")
					},
				})
				err := cmdv2.RunCreate(ctx, "my-org", &cmdv2.CreateClusterParams{Name: "my-db", OrgSlug: "my-org", Plan: "basic"}, &cmdv2.CreatePlanDisplay{Name: "Basic"})
				if tc.wantErr != "" {
					require.EqualError(t, err, "cluster new-id was created, but waiting for readiness failed: "+tc.wantErr)
				} else {
					require.NoError(t, err)
				}
				if tc.stage == "cancel-poll" || tc.stage == "cancel-wait" {
					require.ErrorIs(t, err, context.Canceled)
					require.Contains(t, stderr.String(), "Error: "+err.Error()+"\n")
				}
				require.Zero(t, credentials, "JSON creation must never fetch credentials")
				wantPolls := 1
				if tc.stage == "last-poll" {
					wantPolls = 2
				}
				require.Equal(t, wantPolls, polls)
				require.Equal(t, fmt.Sprintf(`{
    "id": "new-id",
    "name": "my-db",
    "organization": {
        "Name": "",
        "Slug": "my-org"
    },
    "region": "iad",
    "plan": "basic",
    "status": %q
}
`, tc.wantStatus), stdout.String())
				require.Contains(t, stderr.String(), "Waiting for cluster")
			})
		})
	}
}
