package v2

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
		name string
		call func(context.Context, *Client) error
	}{
		{"get cluster", func(ctx context.Context, c *Client) error {
			_, err := c.GetCluster(ctx, "org", "cluster-1")
			return err
		}},
		{"get cluster by ID", func(ctx context.Context, c *Client) error { _, err := c.GetClusterById(ctx, "cluster-1"); return err }},
		{"list regions", func(ctx context.Context, c *Client) error { _, err := c.ListRegions(ctx, "org"); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, body := range []string{"{}", "{"} {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
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

func TestClusterLookupFailureKeepsResourceAndStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer server.Close()
	client, ctx, err := setupTestClient(server)
	require.NoError(t, err)
	_, err = client.GetClusterById(ctx, "cluster-1")
	require.EqualError(t, err, "failed to get cluster cluster-1 (status 500)")
}
