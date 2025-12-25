package mock

import (
	"context"
	"time"

	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
)

var _ uiexutil.Client = (*UiexClient)(nil)

// UiexClient implements the uiexutil.Client interface for testing
type UiexClient struct {
	ListOrganizationsFunc                  func(ctx context.Context, admin bool) ([]uiex.Organization, error)
	GetOrganizationFunc                    func(ctx context.Context, orgSlug string) (*uiex.Organization, error)
	ListMPGRegionsFunc                     func(ctx context.Context, orgSlug string) (uiex.ListMPGRegionsResponse, error)
	ListManagedClustersFunc                func(ctx context.Context, orgSlug string, deleted bool) (uiex.ListManagedClustersResponse, error)
	GetManagedClusterFunc                  func(ctx context.Context, orgSlug string, id string) (uiex.GetManagedClusterResponse, error)
	GetManagedClusterByIdFunc              func(ctx context.Context, id string) (uiex.GetManagedClusterResponse, error)
	CreateUserFunc                         func(ctx context.Context, id string, input uiex.CreateUserInput) (uiex.CreateUserResponse, error)
	CreateUserWithRoleFunc                 func(ctx context.Context, id string, input uiex.CreateUserWithRoleInput) (uiex.CreateUserWithRoleResponse, error)
	UpdateUserRoleFunc                     func(ctx context.Context, id string, username string, input uiex.UpdateUserRoleInput) (uiex.UpdateUserRoleResponse, error)
	DeleteUserFunc                         func(ctx context.Context, id string, username string) error
	GetUserCredentialsFunc                 func(ctx context.Context, id string, username string) (uiex.GetUserCredentialsResponse, error)
	ListUsersFunc                          func(ctx context.Context, id string) (uiex.ListUsersResponse, error)
	ListDatabasesFunc                      func(ctx context.Context, id string) (uiex.ListDatabasesResponse, error)
	CreateDatabaseFunc                     func(ctx context.Context, id string, input uiex.CreateDatabaseInput) (uiex.CreateDatabaseResponse, error)
	CreateClusterFunc                      func(ctx context.Context, input uiex.CreateClusterInput) (uiex.CreateClusterResponse, error)
	DestroyClusterFunc                     func(ctx context.Context, orgSlug string, id string) error
	ListManagedClusterBackupsFunc          func(ctx context.Context, clusterID string) (uiex.ListManagedClusterBackupsResponse, error)
	CreateManagedClusterBackupFunc         func(ctx context.Context, clusterID string, input uiex.CreateManagedClusterBackupInput) (uiex.CreateManagedClusterBackupResponse, error)
	RestoreManagedClusterBackupFunc        func(ctx context.Context, clusterID string, input uiex.RestoreManagedClusterBackupInput) (uiex.RestoreManagedClusterBackupResponse, error)
	CreateAttachmentFunc                   func(ctx context.Context, clusterId string, input uiex.CreateAttachmentInput) (uiex.CreateAttachmentResponse, error)
	CreateBuildFunc                        func(ctx context.Context, in uiex.CreateBuildRequest) (*uiex.BuildResponse, error)
	FinishBuildFunc                        func(ctx context.Context, in uiex.FinishBuildRequest) (*uiex.BuildResponse, error)
	EnsureDepotBuilderFunc                 func(ctx context.Context, in uiex.EnsureDepotBuilderRequest) (*uiex.EnsureDepotBuilderResponse, error)
	CreateFlyManagedBuilderFunc            func(ctx context.Context, orgSlug string, region string) (uiex.CreateFlyManagedBuilderResponse, error)
	GetAllAppsCurrentReleaseTimestampsFunc func(ctx context.Context) (*map[string]time.Time, error)
	ListReleasesFunc                       func(ctx context.Context, appName string, count int) ([]uiex.Release, error)
	GetCurrentReleaseFunc                  func(ctx context.Context, appName string) (*uiex.Release, error)
	CreateReleaseFunc                      func(ctx context.Context, req uiex.CreateReleaseRequest) (*uiex.Release, error)
	UpdateReleaseFunc                      func(ctx context.Context, releaseID, status string, metadata any) (*uiex.Release, error)
}

func (m *UiexClient) ListOrganizations(ctx context.Context, admin bool) ([]uiex.Organization, error) {
	if m.ListOrganizationsFunc != nil {
		return m.ListOrganizationsFunc(ctx, admin)
	}
	return []uiex.Organization{}, nil
}

func (m *UiexClient) GetOrganization(ctx context.Context, orgSlug string) (*uiex.Organization, error) {
	if m.GetOrganizationFunc != nil {
		return m.GetOrganizationFunc(ctx, orgSlug)
	}
	return &uiex.Organization{Slug: orgSlug}, nil
}

func (m *UiexClient) ListMPGRegions(ctx context.Context, orgSlug string) (uiex.ListMPGRegionsResponse, error) {
	if m.ListMPGRegionsFunc != nil {
		return m.ListMPGRegionsFunc(ctx, orgSlug)
	}
	return uiex.ListMPGRegionsResponse{}, nil
}

func (m *UiexClient) ListManagedClusters(ctx context.Context, orgSlug string, deleted bool) (uiex.ListManagedClustersResponse, error) {
	if m.ListManagedClustersFunc != nil {
		return m.ListManagedClustersFunc(ctx, orgSlug, deleted)
	}
	return uiex.ListManagedClustersResponse{}, nil
}

func (m *UiexClient) GetManagedCluster(ctx context.Context, orgSlug string, id string) (uiex.GetManagedClusterResponse, error) {
	if m.GetManagedClusterFunc != nil {
		return m.GetManagedClusterFunc(ctx, orgSlug, id)
	}
	return uiex.GetManagedClusterResponse{}, nil
}

func (m *UiexClient) GetManagedClusterById(ctx context.Context, id string) (uiex.GetManagedClusterResponse, error) {
	if m.GetManagedClusterByIdFunc != nil {
		return m.GetManagedClusterByIdFunc(ctx, id)
	}
	return uiex.GetManagedClusterResponse{}, nil
}

func (m *UiexClient) CreateUser(ctx context.Context, id string, input uiex.CreateUserInput) (uiex.CreateUserResponse, error) {
	if m.CreateUserFunc != nil {
		return m.CreateUserFunc(ctx, id, input)
	}
	return uiex.CreateUserResponse{}, nil
}

func (m *UiexClient) CreateUserWithRole(ctx context.Context, id string, input uiex.CreateUserWithRoleInput) (uiex.CreateUserWithRoleResponse, error) {
	if m.CreateUserWithRoleFunc != nil {
		return m.CreateUserWithRoleFunc(ctx, id, input)
	}
	return uiex.CreateUserWithRoleResponse{}, nil
}

func (m *UiexClient) UpdateUserRole(ctx context.Context, id string, username string, input uiex.UpdateUserRoleInput) (uiex.UpdateUserRoleResponse, error) {
	if m.UpdateUserRoleFunc != nil {
		return m.UpdateUserRoleFunc(ctx, id, username, input)
	}
	return uiex.UpdateUserRoleResponse{}, nil
}

func (m *UiexClient) DeleteUser(ctx context.Context, id string, username string) error {
	if m.DeleteUserFunc != nil {
		return m.DeleteUserFunc(ctx, id, username)
	}
	return nil
}

func (m *UiexClient) GetUserCredentials(ctx context.Context, id string, username string) (uiex.GetUserCredentialsResponse, error) {
	if m.GetUserCredentialsFunc != nil {
		return m.GetUserCredentialsFunc(ctx, id, username)
	}
	return uiex.GetUserCredentialsResponse{}, nil
}

func (m *UiexClient) ListUsers(ctx context.Context, id string) (uiex.ListUsersResponse, error) {
	if m.ListUsersFunc != nil {
		return m.ListUsersFunc(ctx, id)
	}
	return uiex.ListUsersResponse{}, nil
}

func (m *UiexClient) ListDatabases(ctx context.Context, id string) (uiex.ListDatabasesResponse, error) {
	if m.ListDatabasesFunc != nil {
		return m.ListDatabasesFunc(ctx, id)
	}
	return uiex.ListDatabasesResponse{}, nil
}

func (m *UiexClient) CreateDatabase(ctx context.Context, id string, input uiex.CreateDatabaseInput) (uiex.CreateDatabaseResponse, error) {
	if m.CreateDatabaseFunc != nil {
		return m.CreateDatabaseFunc(ctx, id, input)
	}
	return uiex.CreateDatabaseResponse{}, nil
}

func (m *UiexClient) CreateBuild(ctx context.Context, in uiex.CreateBuildRequest) (*uiex.BuildResponse, error) {
	if m.CreateBuildFunc != nil {
		return m.CreateBuildFunc(ctx, in)
	}
	return &uiex.BuildResponse{}, nil
}

func (m *UiexClient) FinishBuild(ctx context.Context, in uiex.FinishBuildRequest) (*uiex.BuildResponse, error) {
	if m.FinishBuildFunc != nil {
		return m.FinishBuildFunc(ctx, in)
	}
	return &uiex.BuildResponse{}, nil
}

func (m *UiexClient) EnsureDepotBuilder(ctx context.Context, in uiex.EnsureDepotBuilderRequest) (*uiex.EnsureDepotBuilderResponse, error) {
	if m.EnsureDepotBuilderFunc != nil {
		return m.EnsureDepotBuilderFunc(ctx, in)
	}
	return &uiex.EnsureDepotBuilderResponse{}, nil
}

func (m *UiexClient) CreateFlyManagedBuilder(ctx context.Context, orgSlug string, region string) (uiex.CreateFlyManagedBuilderResponse, error) {
	if m.CreateFlyManagedBuilderFunc != nil {
		return m.CreateFlyManagedBuilderFunc(ctx, orgSlug, region)
	}
	return uiex.CreateFlyManagedBuilderResponse{}, nil
}

func (m *UiexClient) GetAllAppsCurrentReleaseTimestamps(ctx context.Context) (*map[string]time.Time, error) {
	if m.GetAllAppsCurrentReleaseTimestampsFunc != nil {
		return m.GetAllAppsCurrentReleaseTimestampsFunc(ctx)
	}
	return &map[string]time.Time{}, nil
}

func (m *UiexClient) ListReleases(ctx context.Context, appName string, count int) ([]uiex.Release, error) {
	if m.ListReleasesFunc != nil {
		return m.ListReleasesFunc(ctx, appName, count)
	}
	return []uiex.Release{}, nil
}

func (m *UiexClient) GetCurrentRelease(ctx context.Context, appName string) (*uiex.Release, error) {
	if m.GetCurrentReleaseFunc != nil {
		return m.GetCurrentReleaseFunc(ctx, appName)
	}
	return &uiex.Release{}, nil
}

func (m *UiexClient) CreateRelease(ctx context.Context, req uiex.CreateReleaseRequest) (*uiex.Release, error) {
	if m.CreateReleaseFunc != nil {
		return m.CreateReleaseFunc(ctx, req)
	}
	return &uiex.Release{}, nil
}

func (m *UiexClient) UpdateRelease(ctx context.Context, releaseID, status string, metadata any) (*uiex.Release, error) {
	if m.UpdateReleaseFunc != nil {
		return m.UpdateReleaseFunc(ctx, releaseID, status, metadata)
	}
	return &uiex.Release{}, nil
}

func (m *UiexClient) CreateCluster(ctx context.Context, input uiex.CreateClusterInput) (uiex.CreateClusterResponse, error) {
	if m.CreateClusterFunc != nil {
		return m.CreateClusterFunc(ctx, input)
	}
	return uiex.CreateClusterResponse{}, nil
}

func (m *UiexClient) DestroyCluster(ctx context.Context, orgSlug string, id string) error {
	if m.DestroyClusterFunc != nil {
		return m.DestroyClusterFunc(ctx, orgSlug, id)
	}
	return nil
}

func (m *UiexClient) ListManagedClusterBackups(ctx context.Context, clusterID string) (uiex.ListManagedClusterBackupsResponse, error) {
	if m.ListManagedClusterBackupsFunc != nil {
		return m.ListManagedClusterBackupsFunc(ctx, clusterID)
	}
	return uiex.ListManagedClusterBackupsResponse{}, nil
}

func (m *UiexClient) CreateManagedClusterBackup(ctx context.Context, clusterID string, input uiex.CreateManagedClusterBackupInput) (uiex.CreateManagedClusterBackupResponse, error) {
	if m.CreateManagedClusterBackupFunc != nil {
		return m.CreateManagedClusterBackupFunc(ctx, clusterID, input)
	}
	return uiex.CreateManagedClusterBackupResponse{}, nil
}

func (m *UiexClient) RestoreManagedClusterBackup(ctx context.Context, clusterID string, input uiex.RestoreManagedClusterBackupInput) (uiex.RestoreManagedClusterBackupResponse, error) {
	if m.RestoreManagedClusterBackupFunc != nil {
		return m.RestoreManagedClusterBackupFunc(ctx, clusterID, input)
	}
	return uiex.RestoreManagedClusterBackupResponse{}, nil
}

func (m *UiexClient) CreateAttachment(ctx context.Context, clusterId string, input uiex.CreateAttachmentInput) (uiex.CreateAttachmentResponse, error) {
	if m.CreateAttachmentFunc != nil {
		return m.CreateAttachmentFunc(ctx, clusterId, input)
	}
	return uiex.CreateAttachmentResponse{}, nil
}
