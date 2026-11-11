package v1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClusterResponseErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		call   func(context.Context, *Client) error
	}{
		{"list clusters", 200, func(ctx context.Context, c *Client) error {
			_, err := c.ListManagedClusters(ctx, "org", false)
			return err
		}},
		{"get cluster", 200, func(ctx context.Context, c *Client) error {
			_, err := c.GetManagedCluster(ctx, "org", "cluster-1")
			return err
		}},
		{"get cluster by ID", 200, func(ctx context.Context, c *Client) error {
			_, err := c.GetManagedClusterById(ctx, "cluster-1")
			return err
		}},
		{"create user", 201, func(ctx context.Context, c *Client) error {
			_, err := c.CreateUser(ctx, "cluster-1", CreateUserInput{})
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, body := range []string{`{"ok":true}`, "{"} {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(body))
				}))
				t.Cleanup(server.Close)
				client, ctx, err := setupTestClient(server)
				require.NoError(t, err)
				err = tc.call(ctx, client)
				if body == "{" {
					require.ErrorContains(t, err, "failed to decode response")
					require.NotContains(t, err.Error(), "try again")
				} else {
					require.NoError(t, err)
				}
				require.Equal(t, int32(1), calls.Load())
			}
		})
	}
}

func TestClusterFailuresKeepOperationAndResource(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		get    bool
		want   string
	}{
		{"get status", 500, "", true, "failed to get cluster cluster-1 (status 500)"},
		{"create rejected without details", 201, `{}`, false, "failed to create user in cluster cluster-1: server reported failure without error details (status 201)"},
		{"create error without details", 400, `{}`, false, "failed to create user in cluster cluster-1: server returned no error details (status 400)"},
		{"create malformed error", 400, `{`, false, "failed to decode response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, ctx, err := setupTestClient(server)
			require.NoError(t, err)
			if tc.get {
				_, err = client.GetManagedClusterById(ctx, "cluster-1")
			} else {
				_, err = client.CreateUser(ctx, "cluster-1", CreateUserInput{})
			}
			require.ErrorContains(t, err, tc.want)
			require.NotContains(t, err.Error(), "try again")
		})
	}
}
