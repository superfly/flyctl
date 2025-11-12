package uiexutil

import (
	"context"

	"github.com/superfly/flyctl/internal/uiex"
)

type Client interface {
	// Basic
	ListOrganizations(ctx context.Context, admin bool) ([]uiex.Organization, error)
	GetOrganization(ctx context.Context, orgSlug string) (*uiex.Organization, error)

	// MPGs
	ListMPGRegions(ctx context.Context, orgSlug string) (uiex.ListMPGRegionsResponse, error)
	ListManagedClusters(ctx context.Context, orgSlug string) (uiex.ListManagedClustersResponse, error)
	GetManagedCluster(ctx context.Context, orgSlug string, id string) (uiex.GetManagedClusterResponse, error)
	GetManagedClusterById(ctx context.Context, id string) (uiex.GetManagedClusterResponse, error)
	CreateUser(ctx context.Context, id string, input uiex.CreateUserInput) (uiex.CreateUserResponse, error)
	CreateCluster(ctx context.Context, input uiex.CreateClusterInput) (uiex.CreateClusterResponse, error)
	DestroyCluster(ctx context.Context, orgSlug string, id string) error
	ListManagedClusterBackups(ctx context.Context, clusterID string) (uiex.ListManagedClusterBackupsResponse, error)
	CreateManagedClusterBackup(ctx context.Context, clusterID string, input uiex.CreateManagedClusterBackupInput) (uiex.CreateManagedClusterBackupResponse, error)
	RestoreManagedClusterBackup(ctx context.Context, clusterID string, input uiex.RestoreManagedClusterBackupInput) (uiex.RestoreManagedClusterBackupResponse, error)

	// Builders
	CreateBuild(ctx context.Context, in uiex.CreateBuildRequest) (*uiex.BuildResponse, error)
	FinishBuild(ctx context.Context, in uiex.FinishBuildRequest) (*uiex.BuildResponse, error)
	EnsureDepotBuilder(ctx context.Context, in uiex.EnsureDepotBuilderRequest) (*uiex.EnsureDepotBuilderResponse, error)
	CreateFlyManagedBuilder(ctx context.Context, orgSlug string, region string) (uiex.CreateFlyManagedBuilderResponse, error)

	// Releases
	ListReleases(ctx context.Context, appName string, count int) ([]uiex.Release, error)
	GetCurrentRelease(ctx context.Context, appName string) (*uiex.Release, error)
	CreateRelease(ctx context.Context, req uiex.CreateReleaseRequest) (*uiex.Release, error)
	UpdateRelease(ctx context.Context, releaseID, status string, metadata any) (*uiex.Release, error)
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
