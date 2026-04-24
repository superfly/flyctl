package v1

import (
	"context"
	"net/http"
	"net/url"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiex/mpg"
)

type contextKey struct{}

var clientContextKey = &contextKey{}

type ClientV1 interface {
	ListMPGRegions(ctx context.Context, orgSlug string) (ListMPGRegionsResponse, error)
	ListManagedClusters(ctx context.Context, orgSlug string, deleted bool) (ListManagedClustersResponse, error)
	GetManagedCluster(ctx context.Context, orgSlug string, id string) (GetManagedClusterResponse, error)
	GetManagedClusterById(ctx context.Context, id string) (GetManagedClusterResponse, error)
	CreateUser(ctx context.Context, id string, input CreateUserInput) (CreateUserResponse, error)
	CreateUserWithRole(ctx context.Context, id string, input CreateUserWithRoleInput) (CreateUserWithRoleResponse, error)
	UpdateUserRole(ctx context.Context, id string, username string, input UpdateUserRoleInput) (UpdateUserRoleResponse, error)
	DeleteUser(ctx context.Context, id string, username string) error
	GetUserCredentials(ctx context.Context, id string, username string) (GetUserCredentialsResponse, error)
	ListUsers(ctx context.Context, id string) (ListUsersResponse, error)
	ListDatabases(ctx context.Context, id string) (ListDatabasesResponse, error)
	CreateDatabase(ctx context.Context, id string, input CreateDatabaseInput) (CreateDatabaseResponse, error)
	CreateCluster(ctx context.Context, input CreateClusterInput) (CreateClusterResponse, error)
	DestroyCluster(ctx context.Context, orgSlug string, id string) error
	ListManagedClusterBackups(ctx context.Context, clusterID string) (ListManagedClusterBackupsResponse, error)
	CreateManagedClusterBackup(ctx context.Context, clusterID string, input CreateManagedClusterBackupInput) (CreateManagedClusterBackupResponse, error)
	RestoreManagedClusterBackup(ctx context.Context, clusterID string, input RestoreManagedClusterBackupInput) (RestoreManagedClusterBackupResponse, error)
	CreateAttachment(ctx context.Context, clusterId string, input CreateAttachmentInput) (CreateAttachmentResponse, error)
	DeleteAttachment(ctx context.Context, clusterId string, appName string) (DeleteAttachmentResponse, error)
}

type Client struct {
	*uiex.Client
}

func NewClient(ctx context.Context, opts uiex.NewClientOpts) (*Client, error) {
	uiex, err := uiex.NewWithOptions(ctx, opts)
	if err != nil {
		return nil, err
	}

	return &Client{
		Client: uiex,
	}, nil
}

func (c *Client) BaseURL() *url.URL {
	return c.Client.BaseURL()
}

func (c *Client) HTTPClient() *http.Client {
	return c.Client.HTTPClient()
}

// NewContext derives a Context that carries c from ctx.
func NewContext(ctx context.Context, c ClientV1) context.Context {
	return context.WithValue(ctx, clientContextKey, c)
}

// ClientFromContext returns the ClientV1 ctx carries.
func ClientFromContext(ctx context.Context) ClientV1 {
	c, _ := ctx.Value(clientContextKey).(ClientV1)

	return c
}

type MPGRegion struct {
	Code      string `json:"code"`      // e.g., "fra"
	Available bool   `json:"available"` // Whether this region supports MPG
}

type ListMPGRegionsResponse struct {
	Data []MPGRegion `json:"data"`
}

type ManagedClusterBackup struct {
	Id     string `json:"id"`
	Status string `json:"status"`
	Type   string `json:"type"`
	Start  string `json:"start"`
	Stop   string `json:"stop"`
}

type ListManagedClusterBackupsResponse struct {
	Data []ManagedClusterBackup `json:"data"`
}

type CreateManagedClusterBackupInput struct {
	Type string `json:"type"`
}

type CreateManagedClusterBackupResponse struct {
	Data ManagedClusterBackup `json:"data"`
}

type RestoreManagedClusterBackupInput struct {
	BackupId string `json:"backup_id"`
}

type RestoreManagedClusterBackupResponse struct {
	Data ManagedCluster `json:"data"`
}

type ManagedCluster struct {
	Id            string                          `json:"id"`
	Name          string                          `json:"name"`
	Region        string                          `json:"region"`
	Status        string                          `json:"status"`
	Plan          string                          `json:"plan"`
	Disk          int                             `json:"disk"`
	Replicas      int                             `json:"replicas"`
	Organization  fly.Organization                `json:"organization"`
	IpAssignments mpg.ManagedClusterIpAssignments `json:"ip_assignments"`
	AttachedApps  []mpg.AttachedApp               `json:"attached_apps"`
}

type ListManagedClustersResponse struct {
	Data []ManagedCluster `json:"data"`
}

type GetManagedClusterCredentialsResponse struct {
	Status        string `json:"status"`
	User          string `json:"user"`
	Password      string `json:"password"`
	DBName        string `json:"dbname"`
	ConnectionUri string `json:"pgbouncer_uri"`
}

type GetUserCredentialsResponse struct {
	Data struct {
		User     string `json:"user"`
		Password string `json:"password"`
	} `json:"data"`
}

type GetManagedClusterResponse struct {
	Data        ManagedCluster                       `json:"data"`
	Credentials GetManagedClusterCredentialsResponse `json:"credentials"`
}

type CreateUserInput struct {
	DbName   string `json:"db_name"`
	UserName string `json:"user_name"`
}

type CreateUserResponse struct {
	ConnectionUri string              `json:"connection_uri"`
	Ok            bool                `json:"ok"`
	Errors        uiex.DetailedErrors `json:"errors"`
}

type User struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

type ListUsersResponse struct {
	Data []User `json:"data"`
}

type CreateUserWithRoleInput struct {
	UserName string `json:"user_name"`
	Role     string `json:"role"` // 'schema_admin' | 'writer' | 'reader'
}

type CreateUserWithRoleResponse struct {
	Data User `json:"data"`
}

type UpdateUserRoleInput struct {
	Role string `json:"role"` // 'schema_admin' | 'writer' | 'reader'
}

type UpdateUserRoleResponse struct {
	Data User `json:"data"`
}

type Database struct {
	Name string `json:"name"`
}

type ListDatabasesResponse struct {
	Data []Database `json:"data"`
}

type CreateDatabaseInput struct {
	Name string `json:"name"`
}

type CreateDatabaseResponse struct {
	Data Database `json:"data"`
}

type CreateClusterInput struct {
	Name           string `json:"name"`
	Region         string `json:"region"`
	Plan           string `json:"plan"`
	OrgSlug        string `json:"org_slug"`
	Disk           int    `json:"disk"`
	PostGISEnabled bool   `json:"postgis_enabled"`
	PGMajorVersion string `json:"pg_major_version"`
}

type CreateClusterResponse struct {
	Ok     bool                `json:"ok"`
	Errors uiex.DetailedErrors `json:"errors"`
	Data   struct {
		Id             string                          `json:"id"`
		Name           string                          `json:"name"`
		Status         *string                         `json:"status"`
		Plan           string                          `json:"plan"`
		Environment    *string                         `json:"environment"`
		Region         string                          `json:"region"`
		Organization   fly.Organization                `json:"organization"`
		Replicas       int                             `json:"replicas"`
		Disk           int                             `json:"disk"`
		IpAssignments  mpg.ManagedClusterIpAssignments `json:"ip_assignments"`
		PostGISEnabled bool                            `json:"postgis_enabled"`
	} `json:"data"`
}

type CreateAttachmentInput struct {
	AppName string `json:"app_name"`
}

type CreateAttachmentResponse struct {
	Data struct {
		Id               int64  `json:"id"`
		AppId            int64  `json:"app_id"`
		ManagedServiceId int64  `json:"managed_service_id"`
		AttachedAt       string `json:"attached_at"`
	} `json:"data"`
}

type DeleteAttachmentResponse struct {
	Data struct {
		Message string `json:"message"`
	} `json:"data"`
}
