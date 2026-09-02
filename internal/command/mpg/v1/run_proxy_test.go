package cmdv1

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex/mpg"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	"github.com/superfly/flyctl/wg"
)

type testDialer struct{}

func (*testDialer) State() *wg.WireGuardState { return nil }

func (*testDialer) Config() *wg.Config { return nil }

func (*testDialer) DialContext(context.Context, string, string) (net.Conn, error) {
	return nil, nil
}

func TestProxyParamsIgnoreCredentials(t *testing.T) {
	tests := []struct {
		name        string
		credentials mpgv1.GetManagedClusterCredentialsResponse
	}{
		{
			name: "initializing credentials",
			credentials: mpgv1.GetManagedClusterCredentialsResponse{
				Status: "initializing",
			},
		},
		{
			name: "empty password",
			credentials: mpgv1.GetManagedClusterCredentialsResponse{
				Status: "ready",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := mpgv1.GetManagedClusterResponse{
				Data: mpgv1.ManagedCluster{
					IpAssignments: mpg.ManagedClusterIpAssignments{Direct: "fdaa:0:1234::2"},
				},
				Credentials: tt.credentials,
			}
			dialer := &testDialer{}

			cluster, params, err := proxyParams(&response, nil, "15432", "test-org", "127.0.0.2", dialer)
			require.NoError(t, err)
			assert.Same(t, &response.Data, cluster)
			assert.Equal(t, []string{"15432", "5432"}, params.Ports)
			assert.Equal(t, "test-org", params.OrganizationSlug)
			assert.Equal(t, "127.0.0.2", params.BindAddr)
			assert.Equal(t, "fdaa:0:1234::2", params.RemoteHost)
			assert.Same(t, dialer, params.Dialer)
		})
	}
}

func TestProxyParamsRequireDirectIP(t *testing.T) {
	cluster, params, err := proxyParams(&mpgv1.GetManagedClusterResponse{}, nil, "15432", "test-org", "127.0.0.1", nil)

	require.EqualError(t, err, "error getting cluster IP")
	assert.Nil(t, cluster)
	assert.Nil(t, params)
}

// samplePublicCluster is a representative public Machines API cluster payload.
func samplePublicCluster() flaps.ManagedPostgresCluster {
	return flaps.ManagedPostgresCluster{
		ID:           "mpg-123",
		Name:         "test-cluster",
		Status:       "ready",
		Region:       "ord",
		Plan:         "development",
		DiskSizeGB:   10,
		Replicas:     1,
		Organization: flaps.ManagedPostgresOrganization{Name: "Test Org", Slug: "test-org"},
		Endpoints: flaps.ManagedPostgresEndpoints{
			Primary: struct {
				Direct flaps.ManagedPostgresEndpoint `json:"direct"`
				Pooler flaps.ManagedPostgresEndpoint `json:"pooler"`
			}{
				Direct: flaps.ManagedPostgresEndpoint{Host: "10.0.0.1", Port: 5432},
				Pooler: flaps.ManagedPostgresEndpoint{Host: "10.0.0.2", Port: 6432},
			},
		},
	}
}

// sampleLegacyCluster is a representative legacy MPGv1 cluster payload.
// Direct is a bare address (no port) — the historical shape of this column.
func sampleLegacyCluster() mpgv1.GetManagedClusterResponse {
	return mpgv1.GetManagedClusterResponse{
		Data: mpgv1.ManagedCluster{
			Id: "mpg-123", Name: "test-cluster", Region: "ord", Status: "ready",
			Plan: "development", Disk: 10, Replicas: 1,
			Organization:  fly.Organization{Name: "Test Org", Slug: "test-org"},
			IpAssignments: mpg.ManagedClusterIpAssignments{Direct: "10.0.0.1"},
		},
		Credentials: mpgv1.GetManagedClusterCredentialsResponse{
			Status: "ready", User: "app", Password: "secret",
			DBName: "app", ConnectionUri: "postgres://app:secret@10.0.0.1:5432/app",
		},
	}
}

func TestGetCluster(t *testing.T) {
	tests := []struct {
		name            string
		publicCluster   flaps.ManagedPostgresCluster
		publicErr       error
		legacyResponse  mpgv1.GetManagedClusterResponse
		legacyErr       error
		wantUseLegacy   bool
		wantDirectHost  string
		wantStatus      string
		wantErr         string
		wantPublicCalls int
		wantLegacyCalls int
	}{
		{
			name:            "public success maps to legacy shape",
			publicCluster:   samplePublicCluster(),
			wantUseLegacy:   false,
			wantDirectHost:  "10.0.0.1",
			wantStatus:      "ready",
			wantPublicCalls: 1,
			wantLegacyCalls: 0,
		},
		{
			name: "public success with empty host preserves empty",
			publicCluster: func() flaps.ManagedPostgresCluster {
				c := samplePublicCluster()
				c.Endpoints.Primary.Direct.Host = ""

				return c
			}(),
			wantUseLegacy:   false,
			wantDirectHost:  "",
			wantStatus:      "ready",
			wantPublicCalls: 1,
			wantLegacyCalls: 0,
		},
		{
			name:            "classified 404 falls back to legacy",
			publicCluster:   flaps.ManagedPostgresCluster{},
			publicErr:       flaps.ErrFlapsNotFound,
			legacyResponse:  sampleLegacyCluster(),
			wantUseLegacy:   true,
			wantDirectHost:  "10.0.0.1",
			wantStatus:      "ready",
			wantPublicCalls: 1,
			wantLegacyCalls: 1,
		},
		{
			name:            "classified 404 with legacy failure propagates legacy error",
			publicCluster:   flaps.ManagedPostgresCluster{},
			publicErr:       flaps.ErrFlapsNotFound,
			legacyResponse:  mpgv1.GetManagedClusterResponse{},
			legacyErr:       errors.New("legacy denied"),
			wantErr:         "failed retrieving cluster mpg-123: legacy denied",
			wantPublicCalls: 1,
			wantLegacyCalls: 1,
		},
		{
			name:            "non-404 public error returns without fallback",
			publicCluster:   flaps.ManagedPostgresCluster{},
			publicErr:       errors.New("boom"),
			wantErr:         "failed retrieving cluster mpg-123: boom",
			wantPublicCalls: 1,
			wantLegacyCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publicCalls, legacyCalls := 0, 0
			ctx := flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
				GetManagedPostgresClusterFunc: func(_ context.Context, id string) (flaps.ManagedPostgresCluster, error) {
					publicCalls++
					require.Equal(t, "mpg-123", id)

					return tt.publicCluster, tt.publicErr
				},
			})
			ctx = mpgv1.NewContextWithClient(ctx, &mock.MpgV1Client{
				GetManagedClusterByIdFunc: func(_ context.Context, id string) (mpgv1.GetManagedClusterResponse, error) {
					legacyCalls++
					require.Equal(t, "mpg-123", id)

					return tt.legacyResponse, tt.legacyErr
				},
			})

			got, useLegacy, _, err := getCluster(ctx, "mpg-123")
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				require.Nil(t, got)
				require.Equal(t, tt.wantPublicCalls, publicCalls)
				require.Equal(t, tt.wantLegacyCalls, legacyCalls)

				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantUseLegacy, useLegacy)
			require.NotNil(t, got)
			require.Equal(t, tt.wantDirectHost, got.Data.IpAssignments.Direct)
			require.Equal(t, tt.wantStatus, got.Data.Status)
			require.Equal(t, tt.wantPublicCalls, publicCalls)
			require.Equal(t, tt.wantLegacyCalls, legacyCalls)
		})
	}
}

// TestProxyParamsPublicNeverTouchesCredentials verifies the invariant that
// the proxy code path never resolves credentials: getCluster must only hit
// the cluster lookup endpoint, not the credentials endpoint, on the public-
// success branch. This pins RunProxy against accidentally growing a
// credentials dependency.
func TestProxyParamsPublicNeverTouchesCredentials(t *testing.T) {
	credCalls, legacyCredCalls := 0, 0
	ctx := flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
		GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
			return samplePublicCluster(), nil
		},
		GetManagedPostgresUserCredentialsFunc: func(context.Context, string, string) (flaps.ManagedPostgresUserCredentials, error) {
			credCalls++

			return flaps.ManagedPostgresUserCredentials{}, nil
		},
	})
	ctx = mpgv1.NewContextWithClient(ctx, &mock.MpgV1Client{
		GetUserCredentialsFunc: func(context.Context, string, string) (mpgv1.GetUserCredentialsResponse, error) {
			legacyCredCalls++

			return mpgv1.GetUserCredentialsResponse{}, nil
		},
	})

	response, useLegacy, port, err := getCluster(ctx, "mpg-123")
	require.NoError(t, err)
	require.NotNil(t, response)
	require.False(t, useLegacy)
	require.Equal(t, 0, credCalls, "getCluster (public-success path) must never resolve credentials")
	require.Equal(t, 0, legacyCredCalls, "getCluster must never reach the legacy client on public success")

	// proxyParams does not take a ctx, so it cannot make any HTTP call; it is
	// structurally incapable of leaking credentials. Verify it produces the
	// expected bare-host RemoteHost from the public-converted response.
	cluster, params, err := proxyParams(response, port, "15432", "test-org", "127.0.0.1", nil)
	require.NoError(t, err)
	require.NotNil(t, cluster)
	require.NotNil(t, params)
	require.Equal(t, "10.0.0.1", params.RemoteHost)
	require.Equal(t, 0, credCalls)
	require.Equal(t, 0, legacyCredCalls)
}

// TestProxyParamsPublicShape verifies that on the public path, the
// empty-host case maps to the existing "error getting cluster IP" error and
// the public host maps to the bare RemoteHost (no port).
func TestProxyParamsPublicShape(t *testing.T) {
	t.Run("public host maps to bare RemoteHost", func(t *testing.T) {
		response, port := publicToLegacyClusterResponse(samplePublicCluster())
		_, params, err := proxyParams(&response, port, "15432", "test-org", "127.0.0.1", nil)
		require.NoError(t, err)
		// Public port is 5432; legacy uses bare host with port carried by Ports[1] = "5432".
		require.Equal(t, "10.0.0.1", params.RemoteHost)
		require.Equal(t, []string{"15432", "5432"}, params.Ports)
	})

	t.Run("empty public host returns the legacy 'no IP' error", func(t *testing.T) {
		c := samplePublicCluster()
		c.Endpoints.Primary.Direct.Host = ""
		response, port := publicToLegacyClusterResponse(c)
		_, _, err := proxyParams(&response, port, "15432", "test-org", "127.0.0.1", nil)
		require.EqualError(t, err, "error getting cluster IP")
	})
}

func TestResolveDefaultConnectCredentials(t *testing.T) {
	// The legacy default-user connect path (useLegacy == true) restores the
	// ORIGINAL pre-migration logic: after the credential fetch, the legacy
	// credentials envelope's own Status field (credentials.Status, populated
	// post-fetch and semantically distinct from cluster status) is checked
	// for "initializing"/"error", plus an empty-password fallback. These
	// cases pin that behavior with the original error messages. The public
	// status classifier is intentionally NOT applied here; that is covered
	// by TestResolveConnectCredentialsPublicStatusClassifier below.
	const name = "test-cluster"
	tests := []struct {
		name       string
		clusterSt  string // response.Data.Status — cluster status. NOT consulted on the legacy path.
		credStatus string // credentials.Status — the legacy envelope's own status field (post-fetch).
		credPwd    string // credentials.Password — empty-password fallback when status checks pass.
		err        string
	}{
		{
			// ORIGINAL legacy: credentials envelope says "initializing",
			// refuse with the original pre-migration message.
			name:       "credentials initializing",
			clusterSt:  "ready",
			credStatus: "initializing",
			credPwd:    "secret",
			err:        "cluster is still initializing, wait a bit more",
		},
		{
			// ORIGINAL legacy: credentials envelope says "error" (a real
			// legacy value), refuse with the original "error getting
			// cluster password" message — even with a non-empty password.
			name:       "credentials error",
			clusterSt:  "ready",
			credStatus: "error",
			credPwd:    "secret",
			err:        "error getting cluster password",
		},
		{
			// ORIGINAL legacy: empty password is the empty-password
			// fallback; status field is irrelevant when password is empty.
			name:       "empty password",
			clusterSt:  "ready",
			credStatus: "ready",
			credPwd:    "",
			err:        "error getting cluster password",
		},
		{
			// REGRESSION GUARD: Data.Status="ready" with credentials
			// .Status="initializing" and a non-empty password MUST refuse
			// on the legacy path. Applying the public-only 7-value
			// status classifier to the legacy path (where it does not
			// belong) would silently drop the credentials.Status check
			// and let this case proceed to psql with possibly-invalid
			// credentials. This is the regression this test pins.
			name:       "ready cluster with stale initializing credentials refuses",
			clusterSt:  "ready",
			credStatus: "initializing",
			credPwd:    "secret",
			err:        "cluster is still initializing, wait a bit more",
		},
		{
			// REGRESSION GUARD: Data.Status="ready" with credentials
			// .Status="error" and a non-empty password MUST refuse on
			// the legacy path. Same reasoning as above.
			name:       "ready cluster with stale error credentials refuses",
			clusterSt:  "ready",
			credStatus: "error",
			credPwd:    "secret",
			err:        "error getting cluster password",
		},
		{
			// The cluster status alone ("creating", "failed", etc.) is
			// NOT consulted on the legacy path — only credentials.Status
			// and credentials.Password are. So a "ready"-credentials
			// envelope with a non-empty password proceeds regardless of
			// the cluster status field.
			name:       "creating cluster with ready credentials proceeds",
			clusterSt:  "creating",
			credStatus: "ready",
			credPwd:    "secret",
			err:        "", // no error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := &mpgv1.GetManagedClusterResponse{
				Data: mpgv1.ManagedCluster{Name: name, Status: tt.clusterSt},
				Credentials: mpgv1.GetManagedClusterCredentialsResponse{
					Status:   tt.credStatus,
					Password: tt.credPwd,
				},
			}

			credentials, err := resolveConnectCredentials(context.Background(), response, true, "")

			if tt.err == "" {
				require.NoError(t, err)
				require.NotNil(t, credentials)
				return
			}
			require.EqualError(t, err, tt.err)
			assert.Nil(t, credentials)
		})
	}
}

func TestResolveExplicitUserConnectCredentials(t *testing.T) {
	response := &mpgv1.GetManagedClusterResponse{
		Data: mpgv1.ManagedCluster{
			Id:     "cluster-id",
			Name:   "test-cluster",
			Status: "ready",
		},
		Credentials: mpgv1.GetManagedClusterCredentialsResponse{
			DBName: "default-db",
		},
	}
	client := &mock.MpgV1Client{
		GetUserCredentialsFunc: func(_ context.Context, clusterID string, username string) (mpgv1.GetUserCredentialsResponse, error) {
			assert.Equal(t, "cluster-id", clusterID)
			assert.Equal(t, "app-user", username)

			result := mpgv1.GetUserCredentialsResponse{}
			result.Data.User = "app-user"
			result.Data.Password = "secret"

			return result, nil
		},
	}
	ctx := mpgv1.NewContextWithClient(context.Background(), client)

	credentials, err := resolveConnectCredentials(ctx, response, true, "app-user")

	require.NoError(t, err)
	assert.Equal(t, "app-user", credentials.User)
	assert.Equal(t, "secret", credentials.Password)
	assert.Equal(t, "default-db", credentials.DBName)
}

func TestResolveExplicitUserConnectCredentialsErrors(t *testing.T) {
	const name = "test-cluster"
	t.Run("empty password", func(t *testing.T) {
		client := &mock.MpgV1Client{
			GetUserCredentialsFunc: func(context.Context, string, string) (mpgv1.GetUserCredentialsResponse, error) {
				return mpgv1.GetUserCredentialsResponse{}, nil
			},
		}
		ctx := mpgv1.NewContextWithClient(context.Background(), client)

		response := &mpgv1.GetManagedClusterResponse{
			Data: mpgv1.ManagedCluster{Name: name, Status: "ready"},
		}
		credentials, err := resolveConnectCredentials(ctx, response, true, "app-user")

		require.EqualError(t, err, "error getting user password")
		assert.Nil(t, credentials)
	})

	t.Run("request failure", func(t *testing.T) {
		client := &mock.MpgV1Client{
			GetUserCredentialsFunc: func(context.Context, string, string) (mpgv1.GetUserCredentialsResponse, error) {
				return mpgv1.GetUserCredentialsResponse{}, errors.New("request failed")
			},
		}
		ctx := mpgv1.NewContextWithClient(context.Background(), client)

		response := &mpgv1.GetManagedClusterResponse{
			Data: mpgv1.ManagedCluster{Name: name, Status: "ready"},
		}
		credentials, err := resolveConnectCredentials(ctx, response, true, "app-user")

		require.EqualError(t, err, "failed retrieving credentials for user app-user: request failed")
		assert.Nil(t, credentials)
	})
}

// TestResolveConnectCredentialsPublic covers the new public-API code paths
// for Connect credentials: default-user goes through
// GetManagedPostgresUserCredentials("fly-user") and DBName defaults to "fly-db",
// explicit-user passes the flag value straight through. Data.Status is
// set to "ready" on every response here so the status classifier (which
// runs before any credentials call) does not interfere — the dedicated
// status-classifier tests below exercise every other status and assert
// zero credential calls on refusal.
func TestResolveConnectCredentialsPublic(t *testing.T) {
	response := &mpgv1.GetManagedClusterResponse{
		Data: mpgv1.ManagedCluster{Id: "cluster-id", Name: "test-cluster", Status: "ready"},
	}

	t.Run("default user resolves fly-user from public API", func(t *testing.T) {
		credCalls := 0
		ctx := flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
			GetManagedPostgresUserCredentialsFunc: func(_ context.Context, clusterID, username string) (flaps.ManagedPostgresUserCredentials, error) {
				credCalls++
				require.Equal(t, "cluster-id", clusterID)
				require.Equal(t, "fly-user", username)

				return flaps.ManagedPostgresUserCredentials{Username: "fly-user", Password: "p"}, nil
			},
		})

		credentials, err := resolveConnectCredentials(ctx, response, false, "")
		require.NoError(t, err)
		require.Equal(t, 1, credCalls)
		require.Equal(t, "fly-user", credentials.User)
		require.Equal(t, "p", credentials.Password)
		require.Equal(t, "fly-db", credentials.DBName)
	})

	t.Run("explicit user passes flag value through", func(t *testing.T) {
		credCalls := 0
		ctx := flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
			GetManagedPostgresUserCredentialsFunc: func(_ context.Context, clusterID, username string) (flaps.ManagedPostgresUserCredentials, error) {
				credCalls++
				require.Equal(t, "cluster-id", clusterID)
				require.Equal(t, "alice", username)

				return flaps.ManagedPostgresUserCredentials{Username: "alice", Password: "a"}, nil
			},
		})

		credentials, err := resolveConnectCredentials(ctx, response, false, "alice")
		require.NoError(t, err)
		require.Equal(t, 1, credCalls)
		require.Equal(t, "alice", credentials.User)
		require.Equal(t, "a", credentials.Password)
		// The public path has no envelope DBName; explicit users fall back to
		// defaultMPGDatabase ("fly-db") so that run_connect.go's psql URL
		// construction lands on the plan-required default when neither
		// --database nor an interactive prompt supplies one.
		require.Equal(t, "fly-db", credentials.DBName)
	})

	t.Run("default user empty password mirrors legacy error", func(t *testing.T) {
		ctx := flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
			GetManagedPostgresUserCredentialsFunc: func(context.Context, string, string) (flaps.ManagedPostgresUserCredentials, error) {
				return flaps.ManagedPostgresUserCredentials{Username: "fly-user", Password: ""}, nil
			},
		})

		credentials, err := resolveConnectCredentials(ctx, response, false, "")
		require.EqualError(t, err, "error getting cluster password")
		assert.Nil(t, credentials)
	})

	t.Run("explicit user empty password returns user error", func(t *testing.T) {
		ctx := flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
			GetManagedPostgresUserCredentialsFunc: func(context.Context, string, string) (flaps.ManagedPostgresUserCredentials, error) {
				return flaps.ManagedPostgresUserCredentials{Username: "alice", Password: ""}, nil
			},
		})

		credentials, err := resolveConnectCredentials(ctx, response, false, "alice")
		require.EqualError(t, err, "error getting user password")
		assert.Nil(t, credentials)
	})

	t.Run("default user public credentials error propagates", func(t *testing.T) {
		ctx := flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
			GetManagedPostgresUserCredentialsFunc: func(context.Context, string, string) (flaps.ManagedPostgresUserCredentials, error) {
				return flaps.ManagedPostgresUserCredentials{}, errors.New("denied")
			},
		})

		credentials, err := resolveConnectCredentials(ctx, response, false, "")
		require.EqualError(t, err, "failed retrieving credentials for user fly-user: denied")
		assert.Nil(t, credentials)
	})

	t.Run("explicit user public credentials error propagates", func(t *testing.T) {
		ctx := flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
			GetManagedPostgresUserCredentialsFunc: func(context.Context, string, string) (flaps.ManagedPostgresUserCredentials, error) {
				return flaps.ManagedPostgresUserCredentials{}, errors.New("missing user")
			},
		})

		credentials, err := resolveConnectCredentials(ctx, response, false, "alice")
		require.EqualError(t, err, "failed retrieving credentials for user alice: missing user")
		assert.Nil(t, credentials)
	})
}

// TestPublicToLegacyClusterResponse verifies that the public-API cluster is
// converted to the legacy ui-ex shape, preserving the bare-address column for
// proxyParams and the Status field for the not-ready warning.
func TestPublicToLegacyClusterResponse(t *testing.T) {
	got, _ := publicToLegacyClusterResponse(samplePublicCluster())
	require.Equal(t, "mpg-123", got.Data.Id)
	require.Equal(t, "test-cluster", got.Data.Name)
	require.Equal(t, "ready", got.Data.Status)
	require.Equal(t, "ord", got.Data.Region)
	require.Equal(t, "development", got.Data.Plan)
	require.Equal(t, 10, got.Data.Disk)
	require.Equal(t, 1, got.Data.Replicas)
	require.Equal(t, "Test Org", got.Data.Organization.Name)
	require.Equal(t, "test-org", got.Data.Organization.Slug)
	require.Equal(t, "10.0.0.1", got.Data.IpAssignments.Direct)
}

// TestConnectStatusRefusal pins the pure status classifier for every
// documented public cluster status plus an unknown sentinel. This is the
// "what does the classifier say" table, independent of any I/O — the
// resolveConnectCredentials-level short-circuit is pinned by
// TestResolveConnectCredentialsPublicStatusClassifier below.
func TestConnectStatusRefusal(t *testing.T) {
	const name = "test-cluster"
	tests := []struct {
		status string
		want   string // expected error message; "" means proceed (no error).
	}{
		{"ready", ""},
		{"standby_ready", "cluster " + name + " is a standby replica and cannot be used with fly mpg connect"},
		{"creating", "cluster " + name + " is still being created, wait a bit more"},
		{"deleting", "cluster " + name + " is being deleted and cannot be connected to"},
		{"deleted", "cluster " + name + " has been deleted and cannot be connected to"},
		{"failed", "cluster " + name + " is in a failed state"},
		{"initializing", "cluster " + name + " is not currently ready for connections (status: initializing)"},
		// "error" is not a real public cluster status — it is rejected by
		// the default arm of the classifier as an unrecognized value.
		{"error", `cluster ` + name + ` is in an unrecognized state ("error") and cannot be connected to`},
		// An arbitrary future / unmapped status also falls through to the
		// default arm and surfaces the actual status string quoted.
		{"degraded", `cluster ` + name + ` is in an unrecognized state ("degraded") and cannot be connected to`},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			err := connectStatusRefusal(name, tt.status)
			if tt.want == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tt.want)
		})
	}
}

// TestResolveConnectCredentialsPublicStatusClassifier proves the public-path
// status classifier short-circuits BEFORE any credentials resolution call,
// for both the default-user and explicit-user connect paths. On every
// refused status (the 6 non-ready real statuses + the "error"/"degraded"
// sentinels) it asserts:
//
//   - the exact deliberate refusal message, with the cluster name
//     interpolated;
//   - the credentials endpoint is never called (credCalls == 0).
//
// On the proceed status ("ready") it asserts:
//
//   - no refusal error;
//   - the credentials endpoint IS called exactly once (credCalls == 1).
//
// The classifier applies ONLY to the public path (useLegacy == false). The
// legacy path (useLegacy == true) is intentionally NOT exercised here — it
// uses its original pre-migration credentials.Status / credentials.Password
// post-fetch logic instead, pinned by TestResolveDefaultConnectCredentials
// above. A pure function-level classifier test lives in
// TestConnectStatusRefusal above.
func TestResolveConnectCredentialsPublicStatusClassifier(t *testing.T) {
	const name = "test-cluster"
	const unknownStatus = "degraded"
	tests := []struct {
		status  string
		wantErr string // expected refusal error message; "" means proceed.
	}{
		{"ready", ""},
		{"standby_ready", "cluster " + name + " is a standby replica and cannot be used with fly mpg connect"},
		{"creating", "cluster " + name + " is still being created, wait a bit more"},
		{"deleting", "cluster " + name + " is being deleted and cannot be connected to"},
		{"deleted", "cluster " + name + " has been deleted and cannot be connected to"},
		{"failed", "cluster " + name + " is in a failed state"},
		{"initializing", "cluster " + name + " is not currently ready for connections (status: initializing)"},
		// Sentinel: "error" is NOT a real public cluster status — it is
		// rejected by the default arm of the classifier.
		{"error", `cluster ` + name + ` is in an unrecognized state ("error") and cannot be connected to`},
		// Sentinel: an arbitrary future / unmapped status also fails closed.
		{unknownStatus, `cluster ` + name + ` is in an unrecognized state ("` + unknownStatus + `") and cannot be connected to`},
	}

	for _, tt := range tests {
		t.Run(tt.status+"/default_user_public", func(t *testing.T) {
			c := samplePublicCluster()
			c.Status = tt.status
			c.Name = name
			response, _ := publicToLegacyClusterResponse(c)

			credCalls := 0
			ctx := flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
				GetManagedPostgresUserCredentialsFunc: func(context.Context, string, string) (flaps.ManagedPostgresUserCredentials, error) {
					credCalls++

					return flaps.ManagedPostgresUserCredentials{Username: "fly-user", Password: "p"}, nil
				},
			})

			credentials, err := resolveConnectCredentials(ctx, &response, false, "")
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.Nil(t, credentials)
				require.Equal(t, 0, credCalls, "refusal must short-circuit before any credentials call (default user, public path)")

				return
			}
			require.NoError(t, err)
			require.Equal(t, 1, credCalls, "proceed must hit the credentials endpoint exactly once")
			require.NotNil(t, credentials)
		})

		t.Run(tt.status+"/explicit_user_public", func(t *testing.T) {
			c := samplePublicCluster()
			c.Status = tt.status
			c.Name = name
			response, _ := publicToLegacyClusterResponse(c)

			credCalls := 0
			ctx := flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
				GetManagedPostgresUserCredentialsFunc: func(context.Context, string, string) (flaps.ManagedPostgresUserCredentials, error) {
					credCalls++

					return flaps.ManagedPostgresUserCredentials{Username: "alice", Password: "p"}, nil
				},
			})

			credentials, err := resolveConnectCredentials(ctx, &response, false, "alice")
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.Nil(t, credentials)
				require.Equal(t, 0, credCalls, "refusal must short-circuit before any credentials call (explicit user, public path)")

				return
			}
			require.NoError(t, err)
			require.Equal(t, 1, credCalls, "proceed must hit the credentials endpoint exactly once")
			require.NotNil(t, credentials)
		})
	}
}

// TestProxyParamsPublicNonDefaultPort proves that the public path dials the
// actual advertised port from Endpoints.Primary.Direct.Port instead of the
// legacy hardcoded 5432. The legacy path is also pinned to 5432 here so the
// regression surface is locked down on both sides. The public-zero case is
// pinned to an error so a genuinely-advertised port 0 cannot be silently
// treated like the legacy "no port field" default.
func TestProxyParamsPublicNonDefaultPort(t *testing.T) {
	t.Run("public port 5433 dials 5433, not 5432", func(t *testing.T) {
		c := samplePublicCluster()
		c.Endpoints.Primary.Direct.Port = 5433
		response, port := publicToLegacyClusterResponse(c)

		_, params, err := proxyParams(&response, port, "16380", "test-org", "127.0.0.1", nil)
		require.NoError(t, err)
		require.Equal(t, "10.0.0.1", params.RemoteHost)
		require.Equal(t, []string{"16380", "5433"}, params.Ports)
	})

	t.Run("public port 5432 stays 5432 (no-op for the default)", func(t *testing.T) {
		c := samplePublicCluster()
		c.Endpoints.Primary.Direct.Port = 5432
		response, port := publicToLegacyClusterResponse(c)

		_, params, err := proxyParams(&response, port, "16380", "test-org", "127.0.0.1", nil)
		require.NoError(t, err)
		require.Equal(t, []string{"16380", "5432"}, params.Ports)
	})

	t.Run("legacy port stays hardcoded 5432", func(t *testing.T) {
		response := sampleLegacyCluster()
		// legacy path never calls publicToLegacyClusterResponse, so its port
		// value is always nil — the proxyParams fallback kicks in and "5432"
		// is used exactly as before.
		_, params, err := proxyParams(&response, nil, "16380", "test-org", "127.0.0.1", nil)
		require.NoError(t, err)
		require.Equal(t, []string{"16380", "5432"}, params.Ports)
	})

	t.Run("public port 0 surfaces as an error instead of silently dialing 5432", func(t *testing.T) {
		c := samplePublicCluster()
		c.Endpoints.Primary.Direct.Port = 0
		response, port := publicToLegacyClusterResponse(c)
		// Sanity check: the adapter still produces a non-nil pointer (so the
		// public-vs-legacy distinction is preserved).
		require.NotNil(t, port)
		require.Equal(t, 0, *port)

		cluster, params, err := proxyParams(&response, port, "16380", "test-org", "127.0.0.1", nil)
		require.EqualError(t, err, "error getting cluster port")
		assert.Nil(t, cluster)
		assert.Nil(t, params)
	})
}

// TestPublicToLegacyClusterResponsePortIsPointer verifies the public-to-
// legacy adapter returns the port as a *int pointer (not a bare int). This
// is the structural invariant that lets proxyParams distinguish "public
// path advertised a port (including the invalid 0)" from "legacy path, no
// port info" — without it, a public port of 0 would be indistinguishable
// from the legacy zero value and silently fall back to "5432".
func TestPublicToLegacyClusterResponsePortIsPointer(t *testing.T) {
	t.Run("public port is preserved as a pointer", func(t *testing.T) {
		_, port := publicToLegacyClusterResponse(samplePublicCluster())
		require.NotNil(t, port)
		require.Equal(t, 5432, *port)
	})

	t.Run("public port 0 is preserved as a non-nil pointer to 0", func(t *testing.T) {
		c := samplePublicCluster()
		c.Endpoints.Primary.Direct.Port = 0
		_, port := publicToLegacyClusterResponse(c)
		require.NotNil(t, port)
		require.Equal(t, 0, *port)
	})
}
