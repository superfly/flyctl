package uiexutil

import (
	"context"

	"github.com/superfly/flyctl/internal/uiex"
)

type Client interface {
	// MPGs
	ListMPGRegions(ctx context.Context, orgSlug string) (uiex.ListMPGRegionsResponse, error)
	ListManagedClusters(ctx context.Context, orgSlug string) (uiex.ListManagedClustersResponse, error)
	GetManagedCluster(ctx context.Context, orgSlug string, id string) (uiex.GetManagedClusterResponse, error)
	GetManagedClusterById(ctx context.Context, id string) (uiex.GetManagedClusterResponse, error)
	CreateUser(ctx context.Context, id string, input uiex.CreateUserInput) (uiex.CreateUserResponse, error)
	CreateCluster(ctx context.Context, input uiex.CreateClusterInput) (uiex.CreateClusterResponse, error)
	DestroyCluster(ctx context.Context, orgSlug string, id string) error

	// Builders
	CreateFlyManagedBuilder(ctx context.Context, orgSlug string, region string) (uiex.CreateFlyManagedBuilderResponse, error)

	// Deploys
	CreateDeploy(ctx context.Context, appName string, input uiex.RemoteDeploymentRequest) (<-chan string, error)
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
