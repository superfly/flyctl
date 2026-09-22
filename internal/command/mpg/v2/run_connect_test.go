package cmdv2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

func TestMaybeWarnNotReady(t *testing.T) {
	const name = "test-cluster"
	tests := []struct {
		name     string
		status   string
		wantWarn bool
	}{
		{name: "ready silent", status: "ready", wantWarn: false},
		{name: "creating warns", status: "creating", wantWarn: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cluster := &mpgv2.ManagedCluster{Name: name, Status: tt.status}
			maybeWarnNotReady(&buf, cluster)

			got := buf.String()
			if !tt.wantWarn {
				require.Empty(t, got, "no warning expected for status=%q", tt.status)

				return
			}
			// Check the warning text independently of ANSI color codes.
			require.Contains(t, got, "WARN", "warning must contain the literal 'WARN' marker")
			require.Contains(t, got, "Cluster is not in ready state, currently: "+tt.status)
			require.True(t, bytes.HasSuffix(buf.Bytes(), []byte("\n")), "warning must end with a newline (pre-migration format)")
		})
	}
}

func TestListConnectUsersPublic(t *testing.T) {
	publicCalls, legacyCalls := 0, 0
	ctx := flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
		ListManagedPostgresUsersFunc: func(_ context.Context, id string) ([]flaps.ManagedPostgresUser, error) {
			publicCalls++
			require.Equal(t, "mpg-123", id)

			return []flaps.ManagedPostgresUser{
				{Username: "fly-user", Role: flaps.ManagedPostgresUserRoleSchemaAdmin},
			}, nil
		},
	})
	ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
		ListUsersFunc: func(context.Context, string) (mpgv2.ListUsersResponse, error) {
			legacyCalls++

			return mpgv2.ListUsersResponse{}, nil
		},
	})

	users, err := listConnectUsers(ctx, false, "mpg-123")

	require.NoError(t, err)
	require.Equal(t, 1, publicCalls, "public-resolved cluster must list users through flaps, not legacy")
	require.Zero(t, legacyCalls, "public-resolved cluster must never call the legacy client")
	require.Equal(t, []mpgv2.User{{Name: "fly-user", Role: "schema_admin"}}, users)
}

func TestListConnectUsersLegacy(t *testing.T) {
	publicCalls, legacyCalls := 0, 0
	ctx := flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
		ListManagedPostgresUsersFunc: func(context.Context, string) ([]flaps.ManagedPostgresUser, error) {
			publicCalls++

			return nil, errors.New("must not be called")
		},
	})
	ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
		ListUsersFunc: func(_ context.Context, id string) (mpgv2.ListUsersResponse, error) {
			legacyCalls++
			require.Equal(t, "mpg-123", id)

			return mpgv2.ListUsersResponse{Data: []mpgv2.User{{Name: "legacy-user", Role: "writer"}}}, nil
		},
	})

	users, err := listConnectUsers(ctx, true, "mpg-123")

	require.NoError(t, err)
	require.Equal(t, 1, legacyCalls, "legacy-resolved cluster must list users through the legacy client")
	require.Zero(t, publicCalls, "legacy-resolved cluster must never call the public client for listing")
	require.Equal(t, []mpgv2.User{{Name: "legacy-user", Role: "writer"}}, users)
}

func TestListConnectUsersPublicError(t *testing.T) {
	ctx := flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
		ListManagedPostgresUsersFunc: func(context.Context, string) ([]flaps.ManagedPostgresUser, error) {
			return nil, errors.New("boom")
		},
	})

	users, err := listConnectUsers(ctx, false, "mpg-123")

	require.EqualError(t, err, "boom")
	require.Nil(t, users)
}

func TestListConnectUsersLegacyError(t *testing.T) {
	ctx := mpgv2.NewContextWithClient(context.Background(), &mock.MpgV2Client{
		ListUsersFunc: func(context.Context, string) (mpgv2.ListUsersResponse, error) {
			return mpgv2.ListUsersResponse{}, errors.New("legacy boom")
		},
	})

	users, err := listConnectUsers(ctx, true, "mpg-123")

	require.EqualError(t, err, "legacy boom")
	require.Nil(t, users)
}

func TestListConnectDatabasesPublic(t *testing.T) {
	publicCalls, legacyCalls := 0, 0
	ctx := flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
		ListManagedPostgresDatabasesFunc: func(_ context.Context, id string) ([]flaps.ManagedPostgresDatabase, error) {
			publicCalls++
			require.Equal(t, "mpg-123", id)

			return []flaps.ManagedPostgresDatabase{{Name: "fly-db"}}, nil
		},
	})
	ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
		ListDatabasesFunc: func(context.Context, string) (mpgv2.ListDatabasesResponse, error) {
			legacyCalls++

			return mpgv2.ListDatabasesResponse{}, nil
		},
	})

	databases, err := listConnectDatabases(ctx, false, "mpg-123")

	require.NoError(t, err)
	require.Equal(t, 1, publicCalls, "public-resolved cluster must list databases through flaps, not legacy")
	require.Zero(t, legacyCalls, "public-resolved cluster must never call the legacy client")
	require.Equal(t, []mpgv2.Database{{Name: "fly-db"}}, databases)
}

func TestListConnectDatabasesLegacy(t *testing.T) {
	publicCalls, legacyCalls := 0, 0
	ctx := flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
		ListManagedPostgresDatabasesFunc: func(context.Context, string) ([]flaps.ManagedPostgresDatabase, error) {
			publicCalls++

			return nil, errors.New("must not be called")
		},
	})
	ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
		ListDatabasesFunc: func(_ context.Context, id string) (mpgv2.ListDatabasesResponse, error) {
			legacyCalls++
			require.Equal(t, "mpg-123", id)

			return mpgv2.ListDatabasesResponse{Data: []mpgv2.Database{{Name: "legacy-db"}}}, nil
		},
	})

	databases, err := listConnectDatabases(ctx, true, "mpg-123")

	require.NoError(t, err)
	require.Equal(t, 1, legacyCalls, "legacy-resolved cluster must list databases through the legacy client")
	require.Zero(t, publicCalls, "legacy-resolved cluster must never call the public client for listing")
	require.Equal(t, []mpgv2.Database{{Name: "legacy-db"}}, databases)
}

func TestListConnectDatabasesPublicError(t *testing.T) {
	ctx := flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
		ListManagedPostgresDatabasesFunc: func(context.Context, string) ([]flaps.ManagedPostgresDatabase, error) {
			return nil, errors.New("boom")
		},
	})

	databases, err := listConnectDatabases(ctx, false, "mpg-123")

	require.EqualError(t, err, "boom")
	require.Nil(t, databases)
}

func TestListConnectDatabasesLegacyError(t *testing.T) {
	ctx := mpgv2.NewContextWithClient(context.Background(), &mock.MpgV2Client{
		ListDatabasesFunc: func(context.Context, string) (mpgv2.ListDatabasesResponse, error) {
			return mpgv2.ListDatabasesResponse{}, errors.New("legacy boom")
		},
	})

	databases, err := listConnectDatabases(ctx, true, "mpg-123")

	require.EqualError(t, err, "legacy boom")
	require.Nil(t, databases)
}

// connectTestContext builds a context with interactive IOStreams (stdin and
// stdout both report as a TTY, so io.IsInteractive() is true and RunConnect's
// picker prompts are reachable) plus a fresh flag set with the "username"
// and "database" flags RunConnect reads.
func connectTestContext(t *testing.T) (context.Context, *pflag.FlagSet) {
	t.Helper()

	io, _, _, _ := iostreams.Test()
	io.SetStdinTTY(true)
	io.SetStdoutTTY(true)
	require.True(t, io.IsInteractive())

	ctx := iostreams.NewContext(context.Background(), io)
	flags := pflag.NewFlagSet("connect-test", pflag.ContinueOnError)
	flags.String("username", "", "")
	flags.String("database", "", "")

	return flagctx.NewContext(ctx, flags), flags
}

// refusingMpgV2Client fails the test if any of its methods are called; it
// stands in for the legacy client on paths that must stay purely public.
func refusingMpgV2Client(t *testing.T) *mock.MpgV2Client {
	t.Helper()

	return &mock.MpgV2Client{
		ListUsersFunc: func(context.Context, string) (mpgv2.ListUsersResponse, error) {
			t.Fatal("legacy ListUsers must not be called for a public-resolved cluster")

			return mpgv2.ListUsersResponse{}, nil
		},
		ListDatabasesFunc: func(context.Context, string) (mpgv2.ListDatabasesResponse, error) {
			t.Fatal("legacy ListDatabases must not be called for a public-resolved cluster")

			return mpgv2.ListDatabasesResponse{}, nil
		},
	}
}

// refusingFlapsListClient fails the test if the flaps list-users or
// list-databases methods are called; it stands in for the flaps client on
// paths that must stay purely legacy for listing.
func refusingFlapsListClient(t *testing.T, publicClusterCalls *int, publicErr error) *mock.FlapsClient {
	t.Helper()

	return &mock.FlapsClient{
		GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
			*publicClusterCalls++

			return flaps.ManagedPostgresCluster{}, publicErr
		},
		ListManagedPostgresUsersFunc: func(context.Context, string) ([]flaps.ManagedPostgresUser, error) {
			t.Fatal("flaps ListManagedPostgresUsers must not be called for a legacy-resolved cluster")

			return nil, nil
		},
		ListManagedPostgresDatabasesFunc: func(context.Context, string) ([]flaps.ManagedPostgresDatabase, error) {
			t.Fatal("flaps ListManagedPostgresDatabases must not be called for a legacy-resolved cluster")

			return nil, nil
		},
	}
}

// notFoundClusterErr mirrors the 404 flaps.FlapsError that getCluster treats
// as "fall back to the legacy client" (see TestGetCluster in
// run_proxy_test.go for the same shape).
func notFoundClusterErr() error {
	return fmt.Errorf("wrapped: %w", &flaps.FlapsError{
		ResponseStatusCode: 404,
		OriginalError:      errors.New("not found"),
	})
}

// TestRunConnect_UserPicker_PublicSource proves RunConnect passes the
// useLegacy value getCluster actually resolved (false, here) into the user
// picker: a public-resolved cluster must list users through flaps, never the
// legacy client, and the cluster lookup itself must happen exactly once.
func TestRunConnect_UserPicker_PublicSource(t *testing.T) {
	ctx, _ := connectTestContext(t)

	publicClusterCalls, publicUsersCalls := 0, 0
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
			publicClusterCalls++

			return sampleProxyPublicCluster(), nil
		},
		ListManagedPostgresUsersFunc: func(_ context.Context, id string) ([]flaps.ManagedPostgresUser, error) {
			publicUsersCalls++
			require.Equal(t, "mpg-123", id)

			return nil, errors.New("boom-users")
		},
	})
	ctx = mpgv2.NewContextWithClient(ctx, refusingMpgV2Client(t))

	err := RunConnect(ctx, "mpg-123", "test-org", "15432")

	require.EqualError(t, err, "failed to list users: boom-users")
	require.Equal(t, 1, publicClusterCalls, "getCluster must resolve the cluster exactly once")
	require.Equal(t, 1, publicUsersCalls)
}

// TestRunConnect_UserPicker_LegacySource proves the reverse: a cluster that
// only resolves through the legacy client (public GetManagedPostgresCluster
// 404s) must list users through the legacy client, never flaps.
func TestRunConnect_UserPicker_LegacySource(t *testing.T) {
	ctx, _ := connectTestContext(t)

	publicClusterCalls := 0
	ctx = flapsutil.NewContextWithClient(ctx, refusingFlapsListClient(t, &publicClusterCalls, notFoundClusterErr()))

	legacyClusterCalls, legacyUsersCalls := 0, 0
	ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
		GetClusterByIdFunc: func(_ context.Context, id string) (mpgv2.GetClusterResponse, error) {
			legacyClusterCalls++
			require.Equal(t, "mpg-123", id)

			return sampleProxyLegacyCluster(), nil
		},
		ListUsersFunc: func(_ context.Context, id string) (mpgv2.ListUsersResponse, error) {
			legacyUsersCalls++
			require.Equal(t, "mpg-123", id)

			return mpgv2.ListUsersResponse{}, errors.New("boom-legacy-users")
		},
	})

	err := RunConnect(ctx, "mpg-123", "test-org", "15432")

	require.EqualError(t, err, "failed to list users: boom-legacy-users")
	require.Equal(t, 1, publicClusterCalls, "getCluster must attempt the public lookup exactly once")
	require.Equal(t, 1, legacyClusterCalls, "getCluster must fall back to the legacy lookup exactly once")
	require.Equal(t, 1, legacyUsersCalls)
}

// TestRunConnect_DatabasePicker_PublicSource is the database-picker
// counterpart of TestRunConnect_UserPicker_PublicSource. The username flag
// is set so the user picker is skipped and the database picker is reached.
func TestRunConnect_DatabasePicker_PublicSource(t *testing.T) {
	ctx, flags := connectTestContext(t)
	require.NoError(t, flags.Set("username", "alice"))

	publicClusterCalls, publicDatabasesCalls := 0, 0
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
			publicClusterCalls++

			return sampleProxyPublicCluster(), nil
		},
		ListManagedPostgresDatabasesFunc: func(_ context.Context, id string) ([]flaps.ManagedPostgresDatabase, error) {
			publicDatabasesCalls++
			require.Equal(t, "mpg-123", id)

			return nil, errors.New("boom-databases")
		},
	})
	ctx = mpgv2.NewContextWithClient(ctx, refusingMpgV2Client(t))

	err := RunConnect(ctx, "mpg-123", "test-org", "15432")

	require.EqualError(t, err, "failed to list databases: boom-databases")
	require.Equal(t, 1, publicClusterCalls, "getCluster must resolve the cluster exactly once")
	require.Equal(t, 1, publicDatabasesCalls)
}

// TestRunConnect_DatabasePicker_LegacySource is the database-picker
// counterpart of TestRunConnect_UserPicker_LegacySource.
func TestRunConnect_DatabasePicker_LegacySource(t *testing.T) {
	ctx, flags := connectTestContext(t)
	require.NoError(t, flags.Set("username", "alice"))

	publicClusterCalls := 0
	ctx = flapsutil.NewContextWithClient(ctx, refusingFlapsListClient(t, &publicClusterCalls, notFoundClusterErr()))

	legacyClusterCalls, legacyDatabasesCalls := 0, 0
	ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
		GetClusterByIdFunc: func(_ context.Context, id string) (mpgv2.GetClusterResponse, error) {
			legacyClusterCalls++
			require.Equal(t, "mpg-123", id)

			return sampleProxyLegacyCluster(), nil
		},
		ListDatabasesFunc: func(_ context.Context, id string) (mpgv2.ListDatabasesResponse, error) {
			legacyDatabasesCalls++
			require.Equal(t, "mpg-123", id)

			return mpgv2.ListDatabasesResponse{}, errors.New("boom-legacy-databases")
		},
	})

	err := RunConnect(ctx, "mpg-123", "test-org", "15432")

	require.EqualError(t, err, "failed to list databases: boom-legacy-databases")
	require.Equal(t, 1, publicClusterCalls, "getCluster must attempt the public lookup exactly once")
	require.Equal(t, 1, legacyClusterCalls, "getCluster must fall back to the legacy lookup exactly once")
	require.Equal(t, 1, legacyDatabasesCalls)
}

// TestRunConnect_SkipsPickers_PublicSource proves RunConnect reuses the
// single cluster lookup performed up front (getCluster, via the public
// client here) instead of resolving the cluster a second time when it
// builds connect params. With both the username and database flags set,
// neither interactive picker runs, so the only cluster lookup left is the
// one at the top of RunConnect and the one implicit in credential
// resolution must not add another: connectParamsFromCluster passes the
// already-resolved response through instead of RunConnect calling
// GetMpgConnectParams (which would call getCluster, and so
// GetManagedPostgresCluster, a second time).
func TestRunConnect_SkipsPickers_PublicSource(t *testing.T) {
	ctx, flags := connectTestContext(t)
	require.NoError(t, flags.Set("username", "alice"))
	require.NoError(t, flags.Set("database", "appdb"))

	publicClusterCalls, credentialsCalls := 0, 0
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
			publicClusterCalls++

			return sampleProxyPublicCluster(), nil
		},
		GetManagedPostgresUserCredentialsFunc: func(_ context.Context, id, username string) (flaps.ManagedPostgresUserCredentials, error) {
			credentialsCalls++
			require.Equal(t, "mpg-123", id)
			require.Equal(t, "alice", username)

			return flaps.ManagedPostgresUserCredentials{}, errors.New("boom-credentials")
		},
	})
	ctx = mpgv2.NewContextWithClient(ctx, refusingMpgV2Client(t))

	err := RunConnect(ctx, "mpg-123", "test-org", "15432")

	require.EqualError(t, err, "failed retrieving credentials for user alice: boom-credentials")
	require.Equal(t, 1, publicClusterCalls, "getCluster must resolve the cluster exactly once, even outside the pickers")
	require.Equal(t, 1, credentialsCalls)
}

// TestRunConnect_SkipsPickers_LegacySource is the legacy-resolved
// counterpart of TestRunConnect_SkipsPickers_PublicSource.
func TestRunConnect_SkipsPickers_LegacySource(t *testing.T) {
	ctx, flags := connectTestContext(t)
	require.NoError(t, flags.Set("username", "alice"))
	require.NoError(t, flags.Set("database", "appdb"))

	publicClusterCalls := 0
	ctx = flapsutil.NewContextWithClient(ctx, refusingFlapsListClient(t, &publicClusterCalls, notFoundClusterErr()))

	legacyClusterCalls, credentialsCalls := 0, 0
	ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
		GetClusterByIdFunc: func(_ context.Context, id string) (mpgv2.GetClusterResponse, error) {
			legacyClusterCalls++
			require.Equal(t, "mpg-123", id)

			return sampleProxyLegacyCluster(), nil
		},
		GetUserCredentialsFunc: func(_ context.Context, id, username string) (mpgv2.GetUserCredentialsResponse, error) {
			credentialsCalls++
			require.Equal(t, "mpg-123", id)
			require.Equal(t, "alice", username)

			return mpgv2.GetUserCredentialsResponse{}, errors.New("boom-legacy-credentials")
		},
	})

	err := RunConnect(ctx, "mpg-123", "test-org", "15432")

	require.EqualError(t, err, "failed retrieving credentials for user alice: boom-legacy-credentials")
	require.Equal(t, 1, publicClusterCalls, "getCluster must attempt the public lookup exactly once")
	require.Equal(t, 1, legacyClusterCalls, "getCluster must fall back to the legacy lookup exactly once")
	require.Equal(t, 1, credentialsCalls)
}
