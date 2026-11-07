package cmdv2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/state"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

// attachTestContext builds a context with iostreams, app name, config directory,
// and a fresh empty flag set that callers populate. The IOStreams returned by
// iostreams.Test() are non-interactive, so RunAttach's interactive prompt
// paths are skipped unless callers opt in.
func attachTestContext(t *testing.T) (context.Context, *bytes.Buffer, *bytes.Buffer, *pflag.FlagSet) {
	t.Helper()
	io, _, stdout, stderr := iostreams.Test()
	configDir := t.TempDir()
	t.Setenv("FLY_CONFIG_DIR", configDir)
	flyctl.InitConfig()
	ctx := iostreams.NewContext(context.Background(), io)
	ctx = appconfig.WithName(ctx, "my-app")
	ctx = appconfig.WithConfig(ctx, &appconfig.Config{})
	ctx = state.WithConfigDirectory(ctx, t.TempDir())
	flags := pflag.NewFlagSet("attach-test", pflag.ContinueOnError)

	return flagctx.NewContext(ctx, flags), stdout, stderr, flags
}

// addAttachFlags adds the username, database, and variable-name flags used by
// RunAttach to fs.
func addAttachFlags(fs *pflag.FlagSet) {
	fs.String("username", "", "")
	fs.String("database", "", "")
	fs.String("variable-name", "", "")
}

// minimalAttachFlapsClient returns a mock FlapsClient with all methods needed
// by RunAttach pre-set to succeed or return safe defaults. Callers override
// specific Func fields as needed.
func minimalAttachFlapsClient() *mock.FlapsClient {
	return &mock.FlapsClient{
		ListManagedPostgresUsersFunc: func(_ context.Context, id string) ([]flaps.ManagedPostgresUser, error) {
			return []flaps.ManagedPostgresUser{{Username: "alice", Role: flaps.ManagedPostgresUserRoleWriter}}, nil
		},
		ListManagedPostgresDatabasesFunc: func(_ context.Context, id string) ([]flaps.ManagedPostgresDatabase, error) {
			return []flaps.ManagedPostgresDatabase{{Name: "appdb"}}, nil
		},
		GetManagedPostgresUserCredentialsFunc: func(_ context.Context, id, username string) (flaps.ManagedPostgresUserCredentials, error) {
			return flaps.ManagedPostgresUserCredentials{Username: username, Password: "alice-pass"}, nil
		},
		CreateManagedPostgresAttachmentFunc: func(_ context.Context, id string, req flaps.CreateManagedPostgresAttachmentRequest) (flaps.ManagedPostgresAttachment, error) {
			return flaps.ManagedPostgresAttachment{}, nil
		},
		ListAppSecretsFunc: func(_ context.Context, appName string, version *uint64, show bool) ([]fly.AppSecret, error) {
			return nil, nil
		},
		UpdateAppSecretsFunc: func(_ context.Context, appName string, values map[string]*string) (*fly.UpdateAppSecretsResp, error) {
			return &fly.UpdateAppSecretsResp{Version: 1}, nil
		},
	}
}

// minimalAttachLegacyClient returns a mock MpgV2Client with the cluster
// credentials needed by RunAttach pre-set. Callers override specific Func
// fields as needed.
func minimalAttachLegacyClient() *mock.MpgV2Client {
	return &mock.MpgV2Client{
		GetClusterByIdFunc: func(_ context.Context, id string) (mpgv2.GetClusterResponse, error) {
			return mpgv2.GetClusterResponse{
				Credentials: mpgv2.GetClusterCredentialsResponse{
					User:          "default_user",
					Password:      "default-pass",
					DBName:        "default_db",
					ConnectionUri: "postgres://default_user:default-pass@pooler.fly.dev:5432/default_db",
				},
			}, nil
		},
		GetUserCredentialsFunc: func(_ context.Context, id, username string) (mpgv2.GetUserCredentialsResponse, error) {
			return mpgv2.GetUserCredentialsResponse{
				Data: struct {
					User     string `json:"username"`
					Password string `json:"password"`
				}{User: username, Password: "legacy-pass"},
			}, nil
		},
		CreateAttachmentFunc: func(_ context.Context, clusterID string, input mpgv2.CreateAttachmentInput) (mpgv2.CreateAttachmentResponse, error) {
			return mpgv2.CreateAttachmentResponse{}, nil
		},
	}
}

// wantSecretOutput returns the expected stdout for a successful attach.
func wantSecretOutput(appName, varName, uri string) string {
	return "\nPostgres cluster mpg-123 is being attached to " + appName + "\n" +
		"The following secret was added to " + appName + ":\n" +
		"  " + varName + "=" + uri + "\n"
}

func TestBuildConnectionUri_invalidBase(t *testing.T) {
	_, err := buildConnectionUri("postgres://host:notaport/db", "user", "pass", "database")
	require.Error(t, err)
}

// TestRunAttach_publicSuccess verifies the happy path using all public APIs
func TestRunAttach_publicSuccess(t *testing.T) {
	ctx, stdout, stderr, flags := attachTestContext(t)
	addAttachFlags(flags)
	require.NoError(t, flags.Set("username", "alice"))
	require.NoError(t, flags.Set("database", "appdb"))

	flapsClient := minimalAttachFlapsClient()
	flapsClient.CreateManagedPostgresAttachmentFunc = func(_ context.Context, id string, req flaps.CreateManagedPostgresAttachmentRequest) (flaps.ManagedPostgresAttachment, error) {
		require.Equal(t, "mpg-123", id)
		require.Equal(t, "my-app", req.AppName)

		return flaps.ManagedPostgresAttachment{PostgresClusterID: id, AppName: req.AppName, AttachedAt: "2024-01-01T00:00:00Z"}, nil
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)
	ctx = mpgv2.NewContextWithClient(ctx, minimalAttachLegacyClient())

	err := RunAttach(ctx, "mpg-123")
	require.NoError(t, err)

	// URI: alice's credentials substituted from the public endpoint, path set to appdb.
	wantUri := "postgres://alice:alice-pass@pooler.fly.dev:5432/appdb"
	require.Equal(t, wantSecretOutput("my-app", "DATABASE_URL", wantUri), stdout.String())
	require.Empty(t, stderr.String())
}

// TestRunAttach_usernameUsesClusterDatabase verifies that selecting a user without
// --database still preserves the cluster credential DB name
func TestRunAttach_usernameUsesClusterDatabase(t *testing.T) {
	ctx, stdout, stderr, flags := attachTestContext(t)
	addAttachFlags(flags)
	require.NoError(t, flags.Set("username", "alice"))

	ctx = flapsutil.NewContextWithClient(ctx, minimalAttachFlapsClient())
	ctx = mpgv2.NewContextWithClient(ctx, minimalAttachLegacyClient())

	require.NoError(t, RunAttach(ctx, "mpg-123"))
	require.Equal(t, wantSecretOutput("my-app", "DATABASE_URL", "postgres://alice:alice-pass@pooler.fly.dev:5432/default_db"), stdout.String())
	require.Empty(t, stderr.String())
}

// TestRunAttach_noUsernameDefaultCredentials verifies that when no username is provided,
// the legacy cluster credentials are used directly
func TestRunAttach_noUsernameDefaultCredentials(t *testing.T) {
	ctx, stdout, stderr, flags := attachTestContext(t)
	addAttachFlags(flags)
	// No username or database flags set.

	flapsClient := minimalAttachFlapsClient()
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)
	ctx = mpgv2.NewContextWithClient(ctx, minimalAttachLegacyClient())

	err := RunAttach(ctx, "mpg-123")
	require.NoError(t, err)

	// Default credentials used; database defaults to default_db.
	wantUri := "postgres://default_user:default-pass@pooler.fly.dev:5432/default_db"
	require.Equal(t, wantSecretOutput("my-app", "DATABASE_URL", wantUri), stdout.String())
	require.Empty(t, stderr.String())
}

// TestRunAttach_customVariableName verifies that a custom secret variable name is respected
func TestRunAttach_customVariableName(t *testing.T) {
	ctx, stdout, stderr, flags := attachTestContext(t)
	addAttachFlags(flags)
	require.NoError(t, flags.Set("variable-name", "MY_PG_URL"))
	// No username or database.

	flapsClient := minimalAttachFlapsClient()
	flapsClient.UpdateAppSecretsFunc = func(_ context.Context, appName string, values map[string]*string) (*fly.UpdateAppSecretsResp, error) {
		require.Contains(t, values, "MY_PG_URL")

		return &fly.UpdateAppSecretsResp{Version: 1}, nil
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)
	ctx = mpgv2.NewContextWithClient(ctx, minimalAttachLegacyClient())

	err := RunAttach(ctx, "mpg-123")
	require.NoError(t, err)

	wantUri := "postgres://default_user:default-pass@pooler.fly.dev:5432/default_db"
	require.Equal(t, wantSecretOutput("my-app", "MY_PG_URL", wantUri), stdout.String())
	require.Empty(t, stderr.String())
}

// TestRunAttach_secretAlreadySet verifies that an existing secret prevents overwrite
func TestRunAttach_secretAlreadySet(t *testing.T) {
	ctx, _, _, flags := attachTestContext(t)
	addAttachFlags(flags)

	flapsClient := minimalAttachFlapsClient()
	flapsClient.ListAppSecretsFunc = func(_ context.Context, appName string, version *uint64, show bool) ([]fly.AppSecret, error) {
		require.Equal(t, "my-app", appName)

		return []fly.AppSecret{{Name: "DATABASE_URL"}}, nil
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)
	ctx = mpgv2.NewContextWithClient(ctx, minimalAttachLegacyClient())

	err := RunAttach(ctx, "mpg-123")
	require.ErrorContains(t, err, "already has DATABASE_URL set")
}

// TestRunAttach_invalidConnectionUriError verifies that an empty connection URI is rejected
func TestRunAttach_invalidConnectionUriError(t *testing.T) {
	ctx, _, _, flags := attachTestContext(t)
	addAttachFlags(flags)
	// No username.

	flapsClient := minimalAttachFlapsClient()
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)
	ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
		GetClusterByIdFunc: func(_ context.Context, id string) (mpgv2.GetClusterResponse, error) {
			return mpgv2.GetClusterResponse{
				Credentials: mpgv2.GetClusterCredentialsResponse{
					User:          "default_user",
					Password:      "default-pass",
					DBName:        "default_db",
					ConnectionUri: "", // Empty: triggers error.
				},
			}, nil
		},
	})

	err := RunAttach(ctx, "mpg-123")
	require.Error(t, err)
	require.Contains(t, err.Error(), "connection URI is empty")
}

// TestRunAttach_attachmentWarningOnly verifies that a failed attachment creation produces
// a warning but does not fail the overall attach
func TestRunAttach_attachmentWarningOnly(t *testing.T) {
	ctx, _, stderr, flags := attachTestContext(t)
	addAttachFlags(flags)
	require.NoError(t, flags.Set("username", "alice"))
	require.NoError(t, flags.Set("database", "appdb"))

	flapsClient := minimalAttachFlapsClient()
	flapsClient.CreateManagedPostgresAttachmentFunc = func(_ context.Context, id string, req flaps.CreateManagedPostgresAttachmentRequest) (flaps.ManagedPostgresAttachment, error) {
		return flaps.ManagedPostgresAttachment{}, errors.New("attachment failed")
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)
	ctx = mpgv2.NewContextWithClient(ctx, minimalAttachLegacyClient())

	err := RunAttach(ctx, "mpg-123")
	require.NoError(t, err, "attachment failure should be warning-only")
	require.Contains(t, stderr.String(), "Warning: failed to create attachment record")
}

// TestRunAttach_attachmentFallbackOnNotFound verifies that a classified 404 from the public
// attachment API falls back to the legacy client
func TestRunAttach_attachmentFallbackOnNotFound(t *testing.T) {
	ctx, _, stderr, flags := attachTestContext(t)
	addAttachFlags(flags)
	require.NoError(t, flags.Set("username", "alice"))
	require.NoError(t, flags.Set("database", "appdb"))

	flapsClient := minimalAttachFlapsClient()
	flapsClient.CreateManagedPostgresAttachmentFunc = func(_ context.Context, id string, req flaps.CreateManagedPostgresAttachmentRequest) (flaps.ManagedPostgresAttachment, error) {
		return flaps.ManagedPostgresAttachment{}, flaps.ErrFlapsNotFound
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	legacyCalled := false
	legacyClient := minimalAttachLegacyClient()
	legacyClient.CreateAttachmentFunc = func(_ context.Context, clusterID string, input mpgv2.CreateAttachmentInput) (mpgv2.CreateAttachmentResponse, error) {
		legacyCalled = true
		require.Equal(t, "mpg-123", clusterID)
		require.Equal(t, "my-app", input.AppName)

		return mpgv2.CreateAttachmentResponse{}, nil
	}
	ctx = mpgv2.NewContextWithClient(ctx, legacyClient)

	err := RunAttach(ctx, "mpg-123")
	require.NoError(t, err)
	require.True(t, legacyCalled, "legacy CreateAttachment should be called after public 404")
	require.Empty(t, stderr.String(), "no warning on successful fallback")
}

// TestRunAttach_userCredentialsFallback verifies that GetUserCredentials falls back to the
// legacy client on a classified 404. This exercises the full RunAttach flow with the
// username flag set
func TestRunAttach_userCredentialsFallback(t *testing.T) {
	ctx, stdout, stderr, flags := attachTestContext(t)
	addAttachFlags(flags)
	require.NoError(t, flags.Set("username", "alice"))
	require.NoError(t, flags.Set("database", "appdb"))

	flapsClient := minimalAttachFlapsClient()
	flapsClient.GetManagedPostgresUserCredentialsFunc = func(_ context.Context, id, username string) (flaps.ManagedPostgresUserCredentials, error) {
		return flaps.ManagedPostgresUserCredentials{}, flaps.ErrFlapsNotFound
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	legacyCalled := false
	legacyClient := minimalAttachLegacyClient()
	legacyClient.GetUserCredentialsFunc = func(_ context.Context, id, username string) (mpgv2.GetUserCredentialsResponse, error) {
		legacyCalled = true
		require.Equal(t, "mpg-123", id)
		require.Equal(t, "alice", username)

		return mpgv2.GetUserCredentialsResponse{
			Data: struct {
				User     string `json:"username"`
				Password string `json:"password"`
			}{User: "alice", Password: "legacy-alice-pass"},
		}, nil
	}
	ctx = mpgv2.NewContextWithClient(ctx, legacyClient)

	err := RunAttach(ctx, "mpg-123")
	require.NoError(t, err)
	require.True(t, legacyCalled, "legacy GetUserCredentials should be called after public 404")

	// Verify the legacy password was used in the URI.
	wantUri := "postgres://alice:legacy-alice-pass@pooler.fly.dev:5432/appdb"
	require.Equal(t, wantSecretOutput("my-app", "DATABASE_URL", wantUri), stdout.String())
	require.Empty(t, stderr.String())
}

// TestListDatabasesPublicFirst exercises the extracted helper directly
func TestListDatabasesPublicFirst(t *testing.T) {
	tests := []struct {
		name            string
		publicDatabases []flaps.ManagedPostgresDatabase
		publicErr       error
		legacyDatabases []mpgv2.Database
		legacyErr       error
		wantLegacy      bool
		wantDatabases   []mpgv2.Database
		wantErr         string
	}{
		{
			name:            "uses Machines API",
			publicDatabases: []flaps.ManagedPostgresDatabase{{Name: "appdb"}},
			wantDatabases:   []mpgv2.Database{{Name: "appdb"}},
		},
		{
			name:            "falls back to legacy on public 404",
			publicErr:       flaps.ErrFlapsNotFound,
			legacyDatabases: []mpgv2.Database{{Name: "legacy_db"}},
			wantLegacy:      true,
			wantDatabases:   []mpgv2.Database{{Name: "legacy_db"}},
		},
		{
			name:      "propagates non-404 public error without fallback",
			publicErr: errors.New("internal server error"),
			wantErr:   "internal server error",
		},
		{
			name:       "propagates legacy error after public 404 fallback",
			publicErr:  flaps.ErrFlapsNotFound,
			legacyErr:  errors.New("legacy boom"),
			wantLegacy: true,
			wantErr:    "legacy boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			publicCalls, legacyCalls := 0, 0
			flapsClient := &mock.FlapsClient{
				ListManagedPostgresDatabasesFunc: func(_ context.Context, id string) ([]flaps.ManagedPostgresDatabase, error) {
					publicCalls++
					require.Equal(t, "mpg-123", id)

					return tt.publicDatabases, tt.publicErr
				},
			}
			legacyClient := &mock.MpgV2Client{
				ListDatabasesFunc: func(_ context.Context, id string) (mpgv2.ListDatabasesResponse, error) {
					legacyCalls++
					require.Equal(t, "mpg-123", id)

					return mpgv2.ListDatabasesResponse{Data: tt.legacyDatabases}, tt.legacyErr
				},
			}

			databases, err := listDatabasesPublicFirst(ctx, flapsClient, legacyClient, "mpg-123")
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantDatabases, databases)
			}
			require.Equal(t, 1, publicCalls)
			require.Equal(t, tt.wantLegacy, legacyCalls == 1)
		})
	}
}

// TestCreateUserPublicFirst exercises the extracted helper directly
func TestCreateUserPublicFirst(t *testing.T) {
	tests := []struct {
		name       string
		publicUser flaps.ManagedPostgresUser
		publicErr  error
		legacyUser mpgv2.User
		legacyErr  error
		wantLegacy bool
		wantUser   mpgv2.User
		wantErr    string
	}{
		{
			name:       "uses Machines API",
			publicUser: flaps.ManagedPostgresUser{Username: "newuser", Role: flaps.ManagedPostgresUserRoleWriter},
			wantUser:   mpgv2.User{Name: "newuser", Role: "writer"},
		},
		{
			name:       "falls back to legacy on public 404",
			publicErr:  flaps.ErrFlapsNotFound,
			legacyUser: mpgv2.User{Name: "newuser", Role: "reader"},
			wantLegacy: true,
			wantUser:   mpgv2.User{Name: "newuser", Role: "reader"},
		},
		{
			name:      "propagates non-404 public error without fallback",
			publicErr: errors.New("conflict: user already exists"),
			wantErr:   "conflict: user already exists",
		},
		{
			name:       "propagates legacy error after public 404 fallback",
			publicErr:  flaps.ErrFlapsNotFound,
			legacyErr:  errors.New("legacy boom"),
			wantLegacy: true,
			wantErr:    "legacy boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			publicCalls, legacyCalls := 0, 0
			flapsClient := &mock.FlapsClient{
				CreateManagedPostgresUserFunc: func(_ context.Context, id string, req flaps.CreateManagedPostgresUserRequest) (flaps.ManagedPostgresUser, error) {
					publicCalls++
					require.Equal(t, "mpg-123", id)
					require.Equal(t, "newuser", req.Username)
					require.Equal(t, "writer", req.Role)

					return tt.publicUser, tt.publicErr
				},
			}
			legacyClient := &mock.MpgV2Client{
				CreateUserWithRoleFunc: func(_ context.Context, id string, input mpgv2.CreateUserWithRoleInput) (mpgv2.CreateUserWithRoleResponse, error) {
					legacyCalls++
					require.Equal(t, "mpg-123", id)
					require.Equal(t, "newuser", input.Username)
					require.Equal(t, "writer", input.Role)

					return mpgv2.CreateUserWithRoleResponse{Data: tt.legacyUser}, tt.legacyErr
				},
			}

			user, err := createUserPublicFirst(ctx, flapsClient, legacyClient, "mpg-123", "newuser", "writer")
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantUser, user)
			}
			require.Equal(t, 1, publicCalls)
			require.Equal(t, tt.wantLegacy, legacyCalls == 1)
		})
	}
}

// TestCreateDatabasePublicFirst exercises the extracted helper directly
func TestCreateDatabasePublicFirst(t *testing.T) {
	tests := []struct {
		name       string
		publicErr  error
		legacyErr  error
		wantLegacy bool
		wantErr    string
	}{
		{
			name: "uses Machines API",
		},
		{
			name:       "falls back to legacy on public 404",
			publicErr:  flaps.ErrFlapsNotFound,
			wantLegacy: true,
		},
		{
			name:      "propagates non-404 public error without fallback",
			publicErr: errors.New("conflict: database already exists"),
			wantErr:   "conflict: database already exists",
		},
		{
			name:       "propagates legacy error after public 404 fallback",
			publicErr:  flaps.ErrFlapsNotFound,
			legacyErr:  errors.New("legacy boom"),
			wantLegacy: true,
			wantErr:    "legacy boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			publicCalls, legacyCalls := 0, 0
			flapsClient := &mock.FlapsClient{
				CreateManagedPostgresDatabaseFunc: func(_ context.Context, id string, req flaps.CreateManagedPostgresDatabaseRequest) (flaps.ManagedPostgresDatabase, error) {
					publicCalls++
					require.Equal(t, "mpg-123", id)
					require.Equal(t, "newdb", req.Name)

					return flaps.ManagedPostgresDatabase{Name: "newdb"}, tt.publicErr
				},
			}
			legacyClient := &mock.MpgV2Client{
				CreateDatabaseFunc: func(_ context.Context, id string, input mpgv2.CreateDatabaseInput) error {
					legacyCalls++
					require.Equal(t, "mpg-123", id)
					require.Equal(t, "newdb", input.Name)

					return tt.legacyErr
				},
			}

			err := createDatabasePublicFirst(ctx, flapsClient, legacyClient, "mpg-123", "newdb")
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, 1, publicCalls)
			require.Equal(t, tt.wantLegacy, legacyCalls == 1)
		})
	}
}

// A user limited to databases is attached only to one of them, so the app
// never gets a DATABASE_URL that can't connect.
func TestRunAttach_limitedUser(t *testing.T) {
	for _, tt := range []struct {
		name      string
		databases []string
		database  string
		wantURI   string
		wantErr   string
	}{
		{name: "database in its list", databases: []string{"app1", "appdb"}, database: "appdb", wantURI: "postgres://alice:alice-pass@pooler.fly.dev:5432/appdb"},
		{name: "database outside its list", databases: []string{"app1"}, database: "appdb", wantErr: "user alice can't connect to database appdb; it is limited to app1"},
		{name: "cluster default outside its list", databases: []string{"app1"}, wantErr: "user alice can't connect to database default_db; it is limited to app1"},
		{name: "no databases left", wantErr: "user alice can't connect to any database; give it some with fly mpg users set-databases"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, stdout, _, flags := attachTestContext(t)
			addAttachFlags(flags)
			require.NoError(t, flags.Set("username", "alice"))
			if tt.database != "" {
				require.NoError(t, flags.Set("database", tt.database))
			}

			attached := false
			flapsClient := minimalAttachFlapsClient()
			flapsClient.ListManagedPostgresUsersFunc = func(context.Context, string) ([]flaps.ManagedPostgresUser, error) {
				return []flaps.ManagedPostgresUser{{Username: "alice", Role: flaps.ManagedPostgresUserRoleCustom, Roles: []string{}, DatabasesLimited: true, Databases: tt.databases}}, nil
			}
			flapsClient.CreateManagedPostgresAttachmentFunc = func(context.Context, string, flaps.CreateManagedPostgresAttachmentRequest) (flaps.ManagedPostgresAttachment, error) {
				attached = true

				return flaps.ManagedPostgresAttachment{}, nil
			}
			ctx = flapsutil.NewContextWithClient(ctx, flapsClient)
			ctx = mpgv2.NewContextWithClient(ctx, minimalAttachLegacyClient())

			err := RunAttach(ctx, "mpg-123")
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				require.False(t, attached)
				require.Empty(t, stdout.String())

				return
			}
			require.NoError(t, err)
			require.True(t, attached)
			require.Equal(t, wantSecretOutput("my-app", "DATABASE_URL", tt.wantURI), stdout.String())
		})
	}
}

// attach -u checks the user's database limits, so a users list failure stops
// it with the reason, and a list without limits lets it through.
func TestRunAttach_userLimitsCheck(t *testing.T) {
	t.Run("list failure says why", func(t *testing.T) {
		ctx, stdout, _, flags := attachTestContext(t)
		addAttachFlags(flags)
		require.NoError(t, flags.Set("username", "alice"))
		flapsClient := minimalAttachFlapsClient()
		flapsClient.ListManagedPostgresUsersFunc = func(context.Context, string) ([]flaps.ManagedPostgresUser, error) {
			return nil, &flaps.FlapsError{ResponseStatusCode: 503, OriginalError: errors.New("service unavailable")}
		}
		ctx = flapsutil.NewContextWithClient(ctx, flapsClient)
		ctx = mpgv2.NewContextWithClient(ctx, minimalAttachLegacyClient())

		require.EqualError(t, RunAttach(ctx, "mpg-123"), "couldn't check alice's database limits: service unavailable")
		require.Empty(t, stdout.String())
	})

	t.Run("fallback list without limits attaches", func(t *testing.T) {
		ctx, stdout, _, flags := attachTestContext(t)
		addAttachFlags(flags)
		require.NoError(t, flags.Set("username", "alice"))
		flapsClient := minimalAttachFlapsClient()
		flapsClient.ListManagedPostgresUsersFunc = func(context.Context, string) ([]flaps.ManagedPostgresUser, error) {
			return nil, flaps.ErrFlapsNotFound
		}
		legacyClient := minimalAttachLegacyClient()
		// The body an older postgresv2 sends: no roles or database fields.
		legacyClient.ListUsersFunc = func(context.Context, string) (mpgv2.ListUsersResponse, error) {
			var response mpgv2.ListUsersResponse
			require.NoError(t, json.Unmarshal([]byte(`{"data":[{"name":"alice","role":"custom"}]}`), &response))

			return response, nil
		}
		ctx = flapsutil.NewContextWithClient(ctx, flapsClient)
		ctx = mpgv2.NewContextWithClient(ctx, legacyClient)

		require.NoError(t, RunAttach(ctx, "mpg-123"))
		require.Equal(t, wantSecretOutput("my-app", "DATABASE_URL", "postgres://alice:alice-pass@pooler.fly.dev:5432/default_db"), stdout.String())
	})
}
