package cmdv1

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/mpgutil"
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

			cluster, params, err := proxyParams(&response, mpgutil.DefaultPort, "15432", "test-org", "127.0.0.2", dialer)
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
	cluster, params, err := proxyParams(&mpgv1.GetManagedClusterResponse{}, 0, "15432", "test-org", "127.0.0.1", nil)

	require.EqualError(t, err, "error getting cluster IP")
	assert.Nil(t, cluster)
	assert.Nil(t, params)
}

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
		wantLegacyCalls int
	}{
		{
			name:            "public success maps to legacy shape",
			publicCluster:   samplePublicCluster(),
			wantUseLegacy:   false,
			wantDirectHost:  "10.0.0.1",
			wantStatus:      "ready",
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
			wantLegacyCalls: 0,
		},
		{
			name:          "classified 404 falls back to legacy",
			publicCluster: flaps.ManagedPostgresCluster{},
			publicErr: fmt.Errorf("wrapped: %w", &flaps.FlapsError{
				ResponseStatusCode: 404,
				OriginalError:      errors.New("not found"),
			}),
			legacyResponse:  sampleLegacyCluster(),
			wantUseLegacy:   true,
			wantDirectHost:  "10.0.0.1",
			wantStatus:      "ready",
			wantLegacyCalls: 1,
		},
		{
			name:          "classified 404 with legacy failure propagates legacy error",
			publicCluster: flaps.ManagedPostgresCluster{},
			publicErr: fmt.Errorf("wrapped: %w", &flaps.FlapsError{
				ResponseStatusCode: 404,
				OriginalError:      errors.New("not found"),
			}),
			legacyResponse:  mpgv1.GetManagedClusterResponse{},
			legacyErr:       errors.New("legacy denied"),
			wantErr:         "failed retrieving cluster mpg-123: legacy denied",
			wantLegacyCalls: 1,
		},
		{
			name:            "403 public error returns without fallback",
			publicCluster:   flaps.ManagedPostgresCluster{},
			publicErr:       &flaps.FlapsError{ResponseStatusCode: 403, OriginalError: errors.New("denied")},
			wantErr:         "failed retrieving cluster mpg-123: denied",
			wantLegacyCalls: 0,
		},
		{
			name:            "410 public error returns without fallback",
			publicCluster:   flaps.ManagedPostgresCluster{},
			publicErr:       &flaps.FlapsError{ResponseStatusCode: 410, OriginalError: errors.New("gone")},
			wantErr:         "failed retrieving cluster mpg-123: gone",
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

			got, useLegacy, port, err := getCluster(ctx, "mpg-123")
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				require.Nil(t, got)
				require.Equal(t, 1, publicCalls)
				require.Equal(t, tt.wantLegacyCalls, legacyCalls)

				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantUseLegacy, useLegacy)
			require.NotNil(t, got)
			require.Equal(t, tt.wantDirectHost, got.Data.IpAssignments.Direct)
			require.Equal(t, tt.wantStatus, got.Data.Status)
			require.Equal(t, 1, publicCalls)
			require.Equal(t, tt.wantLegacyCalls, legacyCalls)
			require.Equal(t, 5432, port)
			if tt.wantUseLegacy {
				_, params, err := proxyParams(got, port, "16380", "test-org", "127.0.0.1", nil)
				require.NoError(t, err)
				require.Equal(t, []string{"16380", "5432"}, params.Ports)
			}
		})
	}
}

func TestGetMpgProxyParamsPublicNeverTouchesCredentials(t *testing.T) {
	credCalls, legacyCredCalls := 0, 0
	ctx := flag.NewContext(context.Background(), pflag.NewFlagSet("test", pflag.ContinueOnError))
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
			cluster := samplePublicCluster()
			cluster.Status = "creating"
			cluster.Endpoints.Primary.Direct.Host = ""

			return cluster, nil
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

	cluster, params, err := GetMpgProxyParams(ctx, "15432", "mpg-123", "test-org")
	require.EqualError(t, err, "error getting cluster IP")
	require.Nil(t, cluster)
	require.Nil(t, params)
	require.Zero(t, credCalls)
	require.Zero(t, legacyCredCalls)
}

func TestGetMpgConnectParamsResolvesCredentialsBeforeTunnel(t *testing.T) {
	ctx := flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
		GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
			return samplePublicCluster(), nil
		},
		GetManagedPostgresUserCredentialsFunc: func(context.Context, string, string) (flaps.ManagedPostgresUserCredentials, error) {
			return flaps.ManagedPostgresUserCredentials{Username: mpgutil.DefaultUsername}, nil
		},
	})

	cluster, params, credentials, err := GetMpgConnectParams(ctx, "15432", "", "mpg-123", "test-org")
	require.EqualError(t, err, "error getting cluster password")
	require.Nil(t, cluster)
	require.Nil(t, params)
	require.Nil(t, credentials)
}

func TestResolveDefaultConnectCredentials(t *testing.T) {
	// Legacy readiness comes from the credential envelope, not cluster status.
	const name = "test-cluster"
	tests := []struct {
		name       string
		clusterSt  string // response.Data.Status — cluster status. NOT consulted on the legacy path.
		credStatus string // credentials.Status — the legacy envelope's own status field (post-fetch).
		credPwd    string // credentials.Password — empty-password fallback when status checks pass.
		err        string
	}{
		{
			name:       "empty password",
			clusterSt:  "ready",
			credStatus: "ready",
			credPwd:    "",
			err:        "error getting cluster password",
		},
		{
			name:       "ready cluster with stale initializing credentials refuses",
			clusterSt:  "ready",
			credStatus: "initializing",
			credPwd:    "secret",
			err:        "cluster is still initializing, wait a bit more",
		},
		{
			name:       "ready cluster with stale error credentials refuses",
			clusterSt:  "ready",
			credStatus: "error",
			credPwd:    "secret",
			err:        "error getting cluster password",
		},
		{
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
			Status: "initializing",
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
		require.Equal(t, "fly-db", credentials.DBName)
		require.Equal(t, "postgresql://alice:a@localhost:16380/fly-db", buildConnectURL(credentials, "", "16380"))
		require.Equal(t, "postgresql://alice:a@localhost:16380/app-db", buildConnectURL(credentials, "app-db", "16380"))
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

	t.Run("default user 404 preserves initializing error", func(t *testing.T) {
		ctx := flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
			GetManagedPostgresUserCredentialsFunc: func(context.Context, string, string) (flaps.ManagedPostgresUserCredentials, error) {
				return flaps.ManagedPostgresUserCredentials{}, fmt.Errorf("wrapped: %w", &flaps.FlapsError{ResponseStatusCode: 404, OriginalError: errors.New("not found")})
			},
		})

		credentials, err := resolveConnectCredentials(ctx, response, false, "")
		require.EqualError(t, err, "cluster is still initializing, wait a bit more")
		assert.Nil(t, credentials)
	})

	t.Run("explicit user 404 remains a user error", func(t *testing.T) {
		ctx := flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
			GetManagedPostgresUserCredentialsFunc: func(context.Context, string, string) (flaps.ManagedPostgresUserCredentials, error) {
				return flaps.ManagedPostgresUserCredentials{}, fmt.Errorf("wrapped: %w", &flaps.FlapsError{ResponseStatusCode: 404, OriginalError: errors.New("missing user")})
			},
		})

		credentials, err := resolveConnectCredentials(ctx, response, false, "alice")
		require.EqualError(t, err, "failed retrieving credentials for user alice: wrapped: missing user")
		assert.Nil(t, credentials)
	})
}

func TestResolveConnectCredentialsPublicNonReady(t *testing.T) {
	const name = "test-cluster"
	tests := []struct {
		status string
	}{
		{"creating"},
		{"degraded"},
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
			require.NoError(t, err, "non-ready cluster must proceed to credential resolution")
			require.Equal(t, 1, credCalls, "must hit the credentials endpoint exactly once")
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
			require.NoError(t, err, "non-ready cluster must proceed to credential resolution")
			require.Equal(t, 1, credCalls, "must hit the credentials endpoint exactly once")
			require.NotNil(t, credentials)
		})
	}
}

func TestProxyParamsPublicPorts(t *testing.T) {
	for _, port := range []int{1, 5433, 65535} {
		t.Run(strconv.Itoa(port), func(t *testing.T) {
			c := samplePublicCluster()
			c.Endpoints.Primary.Direct.Port = port
			response, advertisedPort := publicToLegacyClusterResponse(c)

			_, params, err := proxyParams(&response, advertisedPort, "0", "test-org", "127.0.0.1", nil)
			require.NoError(t, err)
			require.Equal(t, "10.0.0.1", params.RemoteHost)
			require.Equal(t, []string{"0", strconv.Itoa(port)}, params.Ports)
		})
	}
}

func TestGetMpgProxyParamsRejectsInvalidPublicPort(t *testing.T) {
	for _, port := range []int{0, -1, 65536} {
		t.Run(strconv.Itoa(port), func(t *testing.T) {
			c := samplePublicCluster()
			c.Endpoints.Primary.Direct.Port = port
			ctx := flag.NewContext(context.Background(), pflag.NewFlagSet("test", pflag.ContinueOnError))
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
					return c, nil
				},
			})
			// No legacy or tunnel client: invalid public endpoints must stop before either.
			cluster, params, err := GetMpgProxyParams(ctx, "0", "mpg-123", "test-org")
			require.EqualError(t, err, fmt.Sprintf("invalid cluster port %d: must be between 1 and 65535", c.Endpoints.Primary.Direct.Port))
			require.Nil(t, cluster)
			require.Nil(t, params)
		})
	}
}
