package cmdv2

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex/mpg"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
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
		credentials mpgv2.GetClusterCredentialsResponse
	}{
		{
			name: "initializing credentials",
			credentials: mpgv2.GetClusterCredentialsResponse{
				Status: "initializing",
			},
		},
		{
			name: "empty password",
			credentials: mpgv2.GetClusterCredentialsResponse{
				Status: "ready",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := mpgv2.GetClusterResponse{
				Data: mpgv2.ManagedCluster{
					IpAssignments: mpg.ManagedClusterIpAssignments{Direct: "fdaa:0:1234::2"},
				},
				Credentials: tt.credentials,
			}
			dialer := &testDialer{}

			cluster, params, err := proxyParams(&response, "15432", "test-org", "127.0.0.2", dialer)
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
	cluster, params, err := proxyParams(&mpgv2.GetClusterResponse{}, "15432", "test-org", "127.0.0.1", nil)

	require.EqualError(t, err, "error getting cluster IP")
	assert.Nil(t, cluster)
	assert.Nil(t, params)
}

func TestResolveDefaultConnectCredentials(t *testing.T) {
	tests := []struct {
		name        string
		credentials mpgv2.GetClusterCredentialsResponse
		err         string
	}{
		{
			name:        "initializing",
			credentials: mpgv2.GetClusterCredentialsResponse{Status: "initializing"},
			err:         "cluster is still initializing, wait a bit more",
		},
		{
			name:        "error status",
			credentials: mpgv2.GetClusterCredentialsResponse{Status: "error", Password: "password"},
			err:         "error getting cluster password",
		},
		{
			name:        "empty password",
			credentials: mpgv2.GetClusterCredentialsResponse{Status: "ready"},
			err:         "error getting cluster password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := &mpgv2.GetClusterResponse{Credentials: tt.credentials}

			credentials, err := resolveConnectCredentials(context.Background(), response, "")

			require.EqualError(t, err, tt.err)
			assert.Nil(t, credentials)
		})
	}
}

func TestResolveExplicitUserConnectCredentials(t *testing.T) {
	response := &mpgv2.GetClusterResponse{
		Data: mpgv2.ManagedCluster{Id: "cluster-id"},
		Credentials: mpgv2.GetClusterCredentialsResponse{
			Status: "initializing",
			DBName: "default-db",
		},
	}
	client := &mock.MpgV2Client{
		GetUserCredentialsFunc: func(_ context.Context, clusterID string, username string) (mpgv2.GetUserCredentialsResponse, error) {
			assert.Equal(t, "cluster-id", clusterID)
			assert.Equal(t, "app-user", username)

			result := mpgv2.GetUserCredentialsResponse{}
			result.Data.User = "app-user"
			result.Data.Password = "secret"

			return result, nil
		},
	}
	ctx := mpgv2.NewContextWithClient(context.Background(), client)

	credentials, err := resolveConnectCredentials(ctx, response, "app-user")

	require.NoError(t, err)
	assert.Equal(t, "app-user", credentials.User)
	assert.Equal(t, "secret", credentials.Password)
	assert.Equal(t, "default-db", credentials.DBName)
}

func TestResolveExplicitUserConnectCredentialsErrors(t *testing.T) {
	t.Run("empty password", func(t *testing.T) {
		client := &mock.MpgV2Client{
			GetUserCredentialsFunc: func(context.Context, string, string) (mpgv2.GetUserCredentialsResponse, error) {
				return mpgv2.GetUserCredentialsResponse{}, nil
			},
		}
		ctx := mpgv2.NewContextWithClient(context.Background(), client)

		credentials, err := resolveConnectCredentials(ctx, &mpgv2.GetClusterResponse{}, "app-user")

		require.EqualError(t, err, "error getting user password")
		assert.Nil(t, credentials)
	})

	t.Run("request failure", func(t *testing.T) {
		client := &mock.MpgV2Client{
			GetUserCredentialsFunc: func(context.Context, string, string) (mpgv2.GetUserCredentialsResponse, error) {
				return mpgv2.GetUserCredentialsResponse{}, errors.New("request failed")
			},
		}
		ctx := mpgv2.NewContextWithClient(context.Background(), client)

		credentials, err := resolveConnectCredentials(ctx, &mpgv2.GetClusterResponse{}, "app-user")

		require.EqualError(t, err, "failed retrieving credentials for user app-user: request failed")
		assert.Nil(t, credentials)
	})
}
