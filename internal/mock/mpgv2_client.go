package mock

import (
	"context"

	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
)

var _ mpgv2.ClientV2 = (*MpgV2Client)(nil)

// MpgV2Client implements the mpgv2.ClientV1 interface for testing
type MpgV2Client struct {
	ListRegionsFunc          func(ctx context.Context, orgSlug string) (mpgv2.ListRegionsResponse, error)
	GetClusterFunc           func(ctx context.Context, orgSlug string, id string) (mpgv2.GetClusterResponse, error)
	GetClusterByIdFunc       func(ctx context.Context, id string) (mpgv2.GetClusterResponse, error)
	CreateUserWithRoleFunc   func(ctx context.Context, id string, input mpgv2.CreateUserWithRoleInput) (mpgv2.CreateUserWithRoleResponse, error)
	UpdateUserRoleFunc       func(ctx context.Context, id string, username string, input mpgv2.UpdateUserRoleInput) error
	DeleteUserFunc           func(ctx context.Context, id string, username string) error
	GetUserCredentialsFunc   func(ctx context.Context, id string, username string) (mpgv2.GetUserCredentialsResponse, error)
	ListUsersFunc            func(ctx context.Context, id string) (mpgv2.ListUsersResponse, error)
	ListDatabasesFunc        func(ctx context.Context, id string) (mpgv2.ListDatabasesResponse, error)
	CreateDatabaseFunc       func(ctx context.Context, id string, input mpgv2.CreateDatabaseInput) error
	CreateClusterFunc        func(ctx context.Context, input mpgv2.CreateClusterInput) (mpgv2.CreateClusterResponse, error)
	DestroyClusterFunc       func(ctx context.Context, orgSlug string, id string) error
	ListClusterBackupsFunc   func(ctx context.Context, clusterID string) (mpgv2.ListClusterBackupsResponse, error)
	CreateClusterBackupFunc  func(ctx context.Context, clusterID string, input mpgv2.CreateClusterBackupInput) error
	RestoreClusterBackupFunc func(ctx context.Context, clusterID string, input mpgv2.RestoreClusterBackupInput) (mpgv2.RestoreClusterBackupResponse, error)
	CreateAttachmentFunc     func(ctx context.Context, clusterId string, input mpgv2.CreateAttachmentInput) (mpgv2.CreateAttachmentResponse, error)
	DeleteAttachmentFunc     func(ctx context.Context, clusterId string, appName string) (mpgv2.DeleteAttachmentResponse, error)
}

func (m *MpgV2Client) ListRegions(ctx context.Context, orgSlug string) (mpgv2.ListRegionsResponse, error) {
	if m.ListRegionsFunc != nil {
		return m.ListRegionsFunc(ctx, orgSlug)
	}

	return mpgv2.ListRegionsResponse{}, nil
}

func (m *MpgV2Client) GetCluster(ctx context.Context, orgSlug string, id string) (mpgv2.GetClusterResponse, error) {
	if m.GetClusterFunc != nil {
		return m.GetClusterFunc(ctx, orgSlug, id)
	}

	return mpgv2.GetClusterResponse{}, nil
}

func (m *MpgV2Client) GetClusterById(ctx context.Context, id string) (mpgv2.GetClusterResponse, error) {
	if m.GetClusterByIdFunc != nil {
		return m.GetClusterByIdFunc(ctx, id)
	}

	return mpgv2.GetClusterResponse{}, nil
}

func (m *MpgV2Client) CreateUserWithRole(ctx context.Context, id string, input mpgv2.CreateUserWithRoleInput) (mpgv2.CreateUserWithRoleResponse, error) {
	if m.CreateUserWithRoleFunc != nil {
		return m.CreateUserWithRoleFunc(ctx, id, input)
	}

	return mpgv2.CreateUserWithRoleResponse{}, nil
}

func (m *MpgV2Client) UpdateUserRole(ctx context.Context, id string, username string, input mpgv2.UpdateUserRoleInput) error {
	if m.UpdateUserRoleFunc != nil {
		return m.UpdateUserRoleFunc(ctx, id, username, input)
	}

	return nil
}

func (m *MpgV2Client) DeleteUser(ctx context.Context, id string, username string) error {
	if m.DeleteUserFunc != nil {
		return m.DeleteUserFunc(ctx, id, username)
	}

	return nil
}

func (m *MpgV2Client) GetUserCredentials(ctx context.Context, id string, username string) (mpgv2.GetUserCredentialsResponse, error) {
	if m.GetUserCredentialsFunc != nil {
		return m.GetUserCredentialsFunc(ctx, id, username)
	}

	return mpgv2.GetUserCredentialsResponse{}, nil
}

func (m *MpgV2Client) ListUsers(ctx context.Context, id string) (mpgv2.ListUsersResponse, error) {
	if m.ListUsersFunc != nil {
		return m.ListUsersFunc(ctx, id)
	}

	return mpgv2.ListUsersResponse{}, nil
}

func (m *MpgV2Client) ListDatabases(ctx context.Context, id string) (mpgv2.ListDatabasesResponse, error) {
	if m.ListDatabasesFunc != nil {
		return m.ListDatabasesFunc(ctx, id)
	}

	return mpgv2.ListDatabasesResponse{}, nil
}

func (m *MpgV2Client) CreateDatabase(ctx context.Context, id string, input mpgv2.CreateDatabaseInput) error {
	if m.CreateDatabaseFunc != nil {
		return m.CreateDatabaseFunc(ctx, id, input)
	}

	return nil
}

func (m *MpgV2Client) CreateCluster(ctx context.Context, input mpgv2.CreateClusterInput) (mpgv2.CreateClusterResponse, error) {
	if m.CreateClusterFunc != nil {
		return m.CreateClusterFunc(ctx, input)
	}

	return mpgv2.CreateClusterResponse{}, nil
}

func (m *MpgV2Client) DestroyCluster(ctx context.Context, orgSlug string, id string) error {
	if m.DestroyClusterFunc != nil {
		return m.DestroyClusterFunc(ctx, orgSlug, id)
	}

	return nil
}

func (m *MpgV2Client) ListClusterBackups(ctx context.Context, clusterID string) (mpgv2.ListClusterBackupsResponse, error) {
	if m.ListClusterBackupsFunc != nil {
		return m.ListClusterBackupsFunc(ctx, clusterID)
	}

	return mpgv2.ListClusterBackupsResponse{}, nil
}

func (m *MpgV2Client) CreateClusterBackup(ctx context.Context, clusterID string, input mpgv2.CreateClusterBackupInput) error {
	if m.CreateClusterBackupFunc != nil {
		return m.CreateClusterBackupFunc(ctx, clusterID, input)
	}

	return nil
}

func (m *MpgV2Client) RestoreClusterBackup(ctx context.Context, clusterID string, input mpgv2.RestoreClusterBackupInput) (mpgv2.RestoreClusterBackupResponse, error) {
	if m.RestoreClusterBackupFunc != nil {
		return m.RestoreClusterBackupFunc(ctx, clusterID, input)
	}

	return mpgv2.RestoreClusterBackupResponse{}, nil
}

func (m *MpgV2Client) CreateAttachment(ctx context.Context, clusterId string, input mpgv2.CreateAttachmentInput) (mpgv2.CreateAttachmentResponse, error) {
	if m.CreateAttachmentFunc != nil {
		return m.CreateAttachmentFunc(ctx, clusterId, input)
	}

	return mpgv2.CreateAttachmentResponse{}, nil
}

func (m *MpgV2Client) DeleteAttachment(ctx context.Context, clusterId string, appName string) (mpgv2.DeleteAttachmentResponse, error) {
	if m.DeleteAttachmentFunc != nil {
		return m.DeleteAttachmentFunc(ctx, clusterId, appName)
	}

	return mpgv2.DeleteAttachmentResponse{}, nil
}
