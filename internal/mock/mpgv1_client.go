package mock

import (
	"context"

	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
)

var _ mpgv1.ClientV1 = (*MpgV1Client)(nil)

// MpgV1Client implements the mpgv1.ClientV1 interface for testing
type MpgV1Client struct {
	ListMPGRegionsFunc              func(ctx context.Context, orgSlug string) (mpgv1.ListMPGRegionsResponse, error)
	ListManagedClustersFunc         func(ctx context.Context, orgSlug string, deleted bool) (mpgv1.ListManagedClustersResponse, error)
	GetManagedClusterFunc           func(ctx context.Context, orgSlug string, id string) (mpgv1.GetManagedClusterResponse, error)
	GetManagedClusterByIdFunc       func(ctx context.Context, id string) (mpgv1.GetManagedClusterResponse, error)
	CreateUserFunc                  func(ctx context.Context, id string, input mpgv1.CreateUserInput) (mpgv1.CreateUserResponse, error)
	CreateUserWithRoleFunc          func(ctx context.Context, id string, input mpgv1.CreateUserWithRoleInput) (mpgv1.CreateUserWithRoleResponse, error)
	UpdateUserRoleFunc              func(ctx context.Context, id string, username string, input mpgv1.UpdateUserRoleInput) (mpgv1.UpdateUserRoleResponse, error)
	DeleteUserFunc                  func(ctx context.Context, id string, username string) error
	GetUserCredentialsFunc          func(ctx context.Context, id string, username string) (mpgv1.GetUserCredentialsResponse, error)
	ListUsersFunc                   func(ctx context.Context, id string) (mpgv1.ListUsersResponse, error)
	ListDatabasesFunc               func(ctx context.Context, id string) (mpgv1.ListDatabasesResponse, error)
	CreateDatabaseFunc              func(ctx context.Context, id string, input mpgv1.CreateDatabaseInput) (mpgv1.CreateDatabaseResponse, error)
	CreateClusterFunc               func(ctx context.Context, input mpgv1.CreateClusterInput) (mpgv1.CreateClusterResponse, error)
	DestroyClusterFunc              func(ctx context.Context, orgSlug string, id string) error
	ListManagedClusterBackupsFunc   func(ctx context.Context, clusterID string) (mpgv1.ListManagedClusterBackupsResponse, error)
	CreateManagedClusterBackupFunc  func(ctx context.Context, clusterID string, input mpgv1.CreateManagedClusterBackupInput) (mpgv1.CreateManagedClusterBackupResponse, error)
	RestoreManagedClusterBackupFunc func(ctx context.Context, clusterID string, input mpgv1.RestoreManagedClusterBackupInput) (mpgv1.RestoreManagedClusterBackupResponse, error)
	CreateAttachmentFunc            func(ctx context.Context, clusterId string, input mpgv1.CreateAttachmentInput) (mpgv1.CreateAttachmentResponse, error)
	DeleteAttachmentFunc            func(ctx context.Context, clusterId string, appName string) (mpgv1.DeleteAttachmentResponse, error)
}

func (m *MpgV1Client) ListMPGRegions(ctx context.Context, orgSlug string) (mpgv1.ListMPGRegionsResponse, error) {
	if m.ListMPGRegionsFunc != nil {
		return m.ListMPGRegionsFunc(ctx, orgSlug)
	}

	return mpgv1.ListMPGRegionsResponse{}, nil
}

func (m *MpgV1Client) ListManagedClusters(ctx context.Context, orgSlug string, deleted bool) (mpgv1.ListManagedClustersResponse, error) {
	if m.ListManagedClustersFunc != nil {
		return m.ListManagedClustersFunc(ctx, orgSlug, deleted)
	}

	return mpgv1.ListManagedClustersResponse{}, nil
}

func (m *MpgV1Client) GetManagedCluster(ctx context.Context, orgSlug string, id string) (mpgv1.GetManagedClusterResponse, error) {
	if m.GetManagedClusterFunc != nil {
		return m.GetManagedClusterFunc(ctx, orgSlug, id)
	}

	return mpgv1.GetManagedClusterResponse{}, nil
}

func (m *MpgV1Client) GetManagedClusterById(ctx context.Context, id string) (mpgv1.GetManagedClusterResponse, error) {
	if m.GetManagedClusterByIdFunc != nil {
		return m.GetManagedClusterByIdFunc(ctx, id)
	}

	return mpgv1.GetManagedClusterResponse{}, nil
}

func (m *MpgV1Client) CreateUser(ctx context.Context, id string, input mpgv1.CreateUserInput) (mpgv1.CreateUserResponse, error) {
	if m.CreateUserFunc != nil {
		return m.CreateUserFunc(ctx, id, input)
	}

	return mpgv1.CreateUserResponse{}, nil
}

func (m *MpgV1Client) CreateUserWithRole(ctx context.Context, id string, input mpgv1.CreateUserWithRoleInput) (mpgv1.CreateUserWithRoleResponse, error) {
	if m.CreateUserWithRoleFunc != nil {
		return m.CreateUserWithRoleFunc(ctx, id, input)
	}

	return mpgv1.CreateUserWithRoleResponse{}, nil
}

func (m *MpgV1Client) UpdateUserRole(ctx context.Context, id string, username string, input mpgv1.UpdateUserRoleInput) (mpgv1.UpdateUserRoleResponse, error) {
	if m.UpdateUserRoleFunc != nil {
		return m.UpdateUserRoleFunc(ctx, id, username, input)
	}

	return mpgv1.UpdateUserRoleResponse{}, nil
}

func (m *MpgV1Client) DeleteUser(ctx context.Context, id string, username string) error {
	if m.DeleteUserFunc != nil {
		return m.DeleteUserFunc(ctx, id, username)
	}

	return nil
}

func (m *MpgV1Client) GetUserCredentials(ctx context.Context, id string, username string) (mpgv1.GetUserCredentialsResponse, error) {
	if m.GetUserCredentialsFunc != nil {
		return m.GetUserCredentialsFunc(ctx, id, username)
	}

	return mpgv1.GetUserCredentialsResponse{}, nil
}

func (m *MpgV1Client) ListUsers(ctx context.Context, id string) (mpgv1.ListUsersResponse, error) {
	if m.ListUsersFunc != nil {
		return m.ListUsersFunc(ctx, id)
	}

	return mpgv1.ListUsersResponse{}, nil
}

func (m *MpgV1Client) ListDatabases(ctx context.Context, id string) (mpgv1.ListDatabasesResponse, error) {
	if m.ListDatabasesFunc != nil {
		return m.ListDatabasesFunc(ctx, id)
	}

	return mpgv1.ListDatabasesResponse{}, nil
}

func (m *MpgV1Client) CreateDatabase(ctx context.Context, id string, input mpgv1.CreateDatabaseInput) (mpgv1.CreateDatabaseResponse, error) {
	if m.CreateDatabaseFunc != nil {
		return m.CreateDatabaseFunc(ctx, id, input)
	}

	return mpgv1.CreateDatabaseResponse{}, nil
}

func (m *MpgV1Client) CreateCluster(ctx context.Context, input mpgv1.CreateClusterInput) (mpgv1.CreateClusterResponse, error) {
	if m.CreateClusterFunc != nil {
		return m.CreateClusterFunc(ctx, input)
	}

	return mpgv1.CreateClusterResponse{}, nil
}

func (m *MpgV1Client) DestroyCluster(ctx context.Context, orgSlug string, id string) error {
	if m.DestroyClusterFunc != nil {
		return m.DestroyClusterFunc(ctx, orgSlug, id)
	}

	return nil
}

func (m *MpgV1Client) ListManagedClusterBackups(ctx context.Context, clusterID string) (mpgv1.ListManagedClusterBackupsResponse, error) {
	if m.ListManagedClusterBackupsFunc != nil {
		return m.ListManagedClusterBackupsFunc(ctx, clusterID)
	}

	return mpgv1.ListManagedClusterBackupsResponse{}, nil
}

func (m *MpgV1Client) CreateManagedClusterBackup(ctx context.Context, clusterID string, input mpgv1.CreateManagedClusterBackupInput) (mpgv1.CreateManagedClusterBackupResponse, error) {
	if m.CreateManagedClusterBackupFunc != nil {
		return m.CreateManagedClusterBackupFunc(ctx, clusterID, input)
	}

	return mpgv1.CreateManagedClusterBackupResponse{}, nil
}

func (m *MpgV1Client) RestoreManagedClusterBackup(ctx context.Context, clusterID string, input mpgv1.RestoreManagedClusterBackupInput) (mpgv1.RestoreManagedClusterBackupResponse, error) {
	if m.RestoreManagedClusterBackupFunc != nil {
		return m.RestoreManagedClusterBackupFunc(ctx, clusterID, input)
	}

	return mpgv1.RestoreManagedClusterBackupResponse{}, nil
}

func (m *MpgV1Client) CreateAttachment(ctx context.Context, clusterId string, input mpgv1.CreateAttachmentInput) (mpgv1.CreateAttachmentResponse, error) {
	if m.CreateAttachmentFunc != nil {
		return m.CreateAttachmentFunc(ctx, clusterId, input)
	}

	return mpgv1.CreateAttachmentResponse{}, nil
}

func (m *MpgV1Client) DeleteAttachment(ctx context.Context, clusterId string, appName string) (mpgv1.DeleteAttachmentResponse, error) {
	if m.DeleteAttachmentFunc != nil {
		return m.DeleteAttachmentFunc(ctx, clusterId, appName)
	}

	return mpgv1.DeleteAttachmentResponse{}, nil
}
