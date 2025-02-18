package uiexutil

import (
	"context"

	"github.com/superfly/flyctl/internal/uiex"
)

type Client interface {
	ListManagedClusters(ctx context.Context, orgSlug string) (uiex.ListManagedClustersResponse, error)
	GetManagedCluster(ctx context.Context, orgSlug string, id string) (uiex.GetManagedClusterResponse, error)
}

type contextKey struct{}

var clientContextKey = &contextKey{}

// NewContextWithClient derives a Context that carries c from ctx.
func NewContextWithClient(ctx context.Context, c Client) context.Context {
	return context.WithValue(ctx, clientContextKey, c)
}

// ClientFromContext returns the Client ctx carries.
func ClientFromContext(ctx context.Context) Client {
	c, _ := ctx.Value(clientContextKey).(Client)
	return c
}
