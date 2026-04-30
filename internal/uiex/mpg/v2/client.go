package v2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiex/mpg"
)

type contextKey struct{}

var clientContextKey = &contextKey{}

type ClientV2 interface {
	GetCluster(ctx context.Context, orgSlug string, id string) (GetClusterResponse, error)
	GetClusterById(ctx context.Context, id string) (GetClusterResponse, error)
	ListRegions(ctx context.Context, orgSlug string) (ListRegionsResponse, error)
	CreateUserWithRole(ctx context.Context, id string, input CreateUserWithRoleInput) (CreateUserWithRoleResponse, error)
	UpdateUserRole(ctx context.Context, id string, username string, input UpdateUserRoleInput) error
	DeleteUser(ctx context.Context, id string, username string) error
	GetUserCredentials(ctx context.Context, id string, username string) (GetUserCredentialsResponse, error)
	ListUsers(ctx context.Context, id string) (ListUsersResponse, error)
	ListDatabases(ctx context.Context, id string) (ListDatabasesResponse, error)
	CreateDatabase(ctx context.Context, id string, input CreateDatabaseInput) error
	CreateCluster(ctx context.Context, input CreateClusterInput) (CreateClusterResponse, error)
	DestroyCluster(ctx context.Context, orgSlug string, id string) error
	ListClusterBackups(ctx context.Context, clusterID string) (ListClusterBackupsResponse, error)
	CreateClusterBackup(ctx context.Context, clusterID string, input CreateClusterBackupInput) error
	RestoreClusterBackup(ctx context.Context, clusterID string, input RestoreClusterBackupInput) (RestoreClusterBackupResponse, error)
	CreateAttachment(ctx context.Context, clusterId string, input CreateAttachmentInput) (CreateAttachmentResponse, error)
	DeleteAttachment(ctx context.Context, clusterId string, appName string) (DeleteAttachmentResponse, error)
}

type Client struct {
	*uiex.Client
}

func NewClientWithOptions(ctx context.Context, opts uiex.NewClientOpts) (*Client, error) {
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
func NewContextWithClient(ctx context.Context, c ClientV2) context.Context {
	return context.WithValue(ctx, clientContextKey, c)
}

// ClientFromContext returns the ClientV2 ctx carries.
func ClientFromContext(ctx context.Context) ClientV2 {
	c, _ := ctx.Value(clientContextKey).(ClientV2)

	return c
}

type ManagedCluster struct {
	Id            string                          `json:"id"`
	Name          string                          `json:"name"`
	Status        string                          `json:"status"`
	Engine        string                          `json:"engine"`
	Plan          string                          `json:"plan"`
	Region        string                          `json:"region"`
	Organization  fly.Organization                `json:"organization"`
	MpgdClusterId string                          `json:"mpgd_cluster_id"`
	Disk          int                             `json:"disk"`
	Replicas      int                             `json:"replicas"`
	IpAssignments mpg.ManagedClusterIpAssignments `json:"ip_assignments"`
	AttachedApps  []mpg.AttachedApp               `json:"attached_apps"`
}

type GetClusterCredentialsResponse struct {
	Status        string `json:"status"`
	User          string `json:"user"`
	Password      string `json:"password"`
	DBName        string `json:"dbname"`
	ConnectionUri string `json:"pgbouncer_uri"`
}

type GetClusterResponse struct {
	Data        ManagedCluster                `json:"data"`
	Credentials GetClusterCredentialsResponse `json:"credentials"`
}

type GetUserCredentialsResponse struct {
	Data struct {
		User     string `json:"username"`
		Password string `json:"password"`
	} `json:"data"`
}

type User struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

type ListUsersResponse struct {
	Data []User `json:"data"`
}

type CreateUserWithRoleInput struct {
	Username string `json:"username"`
	Role     string `json:"role"` // 'schema_admin' | 'writer' | 'reader'
}

type CreateUserWithRoleResponse struct {
	Data User `json:"data"`
}

type UpdateUserRoleInput struct {
	Role string `json:"role"` // 'schema_admin' | 'writer' | 'reader'
}

type Region struct {
	Code      string `json:"code"`      // e.g., "fra"
	Available bool   `json:"available"` // Whether this region supports MPG
}

type ListRegionsResponse struct {
	Data []Region `json:"data"`
}

type ListDatabasesResponse struct {
	Data []Database `json:"data"`
}

type Database struct {
	Name string `json:"name"`
}

type CreateDatabaseInput struct {
	Name string `json:"name"`
}

type ClusterBackup struct {
	Id     string `json:"id"`
	Status string `json:"status"`
	Type   string `json:"type"`
	Start  string `json:"start"`
	Stop   string `json:"stop"`
}

type ListClusterBackupsResponse struct {
	Data []ClusterBackup `json:"data"`
}

type CreateClusterBackupInput struct {
	Type string `json:"type"`
}

type RestoreClusterBackupInput struct {
	BackupId string `json:"backup_id"`
}

type RestoreClusterBackupResponse struct {
	Data ManagedCluster `json:"data"`
}

type CreateClusterInput struct {
	Name           string `json:"name"`
	Region         string `json:"region"`
	OrgSlug        string `json:"org_slug"`
	Plan           string `json:"plan"`
	PGMajorVersion string `json:"pg_major_version"`
	StorageInGb    int    `json:"storage_in_gb"`
	PostGISEnabled bool   `json:"postgis_enabled"`
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

func (c *Client) GetCluster(ctx context.Context, orgSlug string, id string) (GetClusterResponse, error) {
	var response GetClusterResponse

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/organizations/%s/postgresv2/%s", c.BaseURL(), orgSlug, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.HTTPClient().Do(req)
	if err != nil {
		return response, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return response, fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusOK:
		if err = json.Unmarshal(body, &response); err != nil {
			return response, fmt.Errorf("failed to decode response, please try again: %w", err)
		}

		return response, nil
	case http.StatusNotFound:
		return response, fmt.Errorf("cluster %s not found in organization %s", id, orgSlug)
	default:
		return response, fmt.Errorf("failed to get cluster (status %d): %s", res.StatusCode, string(body))
	}
}

func (c *Client) GetClusterById(ctx context.Context, id string) (GetClusterResponse, error) {
	var response GetClusterResponse

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgresv2/%s", c.BaseURL(), id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.HTTPClient().Do(req)
	if err != nil {
		return response, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return response, fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusOK:
		if err = json.Unmarshal(body, &response); err != nil {
			return response, fmt.Errorf("failed to decode response, please try again: %w", err)
		}

		return response, nil
	case http.StatusNotFound:
		return response, fmt.Errorf("Cluster %s not found", id)
	default:
		return response, fmt.Errorf("Something went wrong")
	}
}

func (c *Client) ListRegions(ctx context.Context, orgSlug string) (ListRegionsResponse, error) {
	var response ListRegionsResponse

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/organizations/%s/postgresv2/regions", c.BaseURL(), orgSlug)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.HTTPClient().Do(req)
	if err != nil {
		return response, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return response, fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusOK:
		if err = json.Unmarshal(body, &response); err != nil {
			return response, fmt.Errorf("failed to decode response, please try again: %w", err)
		}

		return response, nil
	default:
		return response, fmt.Errorf("failed to list MPG regions (status %d): %s", res.StatusCode, string(body))
	}
}

func (c *Client) ListDatabases(ctx context.Context, id string) (ListDatabasesResponse, error) {
	var response ListDatabasesResponse

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgresv2/%s/databases", c.BaseURL(), id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.HTTPClient().Do(req)
	if err != nil {
		return response, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return response, fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusOK:
		if err = json.Unmarshal(body, &response); err != nil {
			return response, fmt.Errorf("failed to decode response: %w", err)
		}

		return response, nil
	case http.StatusNotFound:
		return response, fmt.Errorf("cluster %s not found", id)
	case http.StatusForbidden:
		return response, fmt.Errorf("access denied: you don't have permission to list databases for cluster %s", id)
	default:
		return response, fmt.Errorf("failed to list databases (status %d): %s", res.StatusCode, string(body))
	}
}

func (c *Client) CreateDatabase(ctx context.Context, id string, input CreateDatabaseInput) error {
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgresv2/%s/databases", c.BaseURL(), id)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(input); err != nil {
		return fmt.Errorf("failed to encode request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.HTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("cluster %s not found", id)
	case http.StatusForbidden:
		return fmt.Errorf("access denied: you don't have permission to create databases for cluster %s", id)
	default:
		return fmt.Errorf("failed to create database (status %d): %s", res.StatusCode, string(body))
	}
}

func (c *Client) CreateUserWithRole(ctx context.Context, id string, input CreateUserWithRoleInput) (CreateUserWithRoleResponse, error) {
	var response CreateUserWithRoleResponse

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgresv2/%s/users", c.BaseURL(), id)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(input); err != nil {
		return response, fmt.Errorf("failed to encode request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.HTTPClient().Do(req)
	if err != nil {
		return response, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return response, fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusOK, http.StatusCreated:
		if err = json.Unmarshal(body, &response); err != nil {
			return response, fmt.Errorf("failed to decode response: %w", err)
		}

		return response, nil
	case http.StatusNotFound:
		return response, fmt.Errorf("cluster %s not found", id)
	case http.StatusForbidden:
		return response, fmt.Errorf("access denied: you don't have permission to create users for cluster %s", id)
	default:
		return response, fmt.Errorf("failed to create user (status %d): %s", res.StatusCode, string(body))
	}
}

func (c *Client) UpdateUserRole(ctx context.Context, id string, username string, input UpdateUserRoleInput) error {
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgresv2/%s/users/%s", c.BaseURL(), id, username)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(input); err != nil {
		return fmt.Errorf("failed to encode request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, &buf)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.HTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("cluster %s or user %s not found", id, username)
	case http.StatusForbidden:
		return fmt.Errorf("access denied: you don't have permission to update users for cluster %s", id)
	default:
		return fmt.Errorf("failed to update user role (status %d): %s", res.StatusCode, string(body))
	}
}

func (c *Client) DeleteUser(ctx context.Context, id string, username string) error {
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgresv2/%s/users/%s", c.BaseURL(), id, username)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.HTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("cluster %s or user %s not found", id, username)
	case http.StatusForbidden:
		return fmt.Errorf("access denied: you don't have permission to delete users for cluster %s", id)
	default:
		return fmt.Errorf("failed to delete user (status %d): %s", res.StatusCode, string(body))
	}
}

func (c *Client) GetUserCredentials(ctx context.Context, id string, username string) (GetUserCredentialsResponse, error) {
	var response GetUserCredentialsResponse

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgresv2/%s/users/%s/credentials", c.BaseURL(), id, username)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.HTTPClient().Do(req)
	if err != nil {
		return response, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return response, fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusOK:
		if err = json.Unmarshal(body, &response); err != nil {
			return response, fmt.Errorf("failed to decode response: %w", err)
		}

		return response, nil
	case http.StatusNotFound:
		return response, fmt.Errorf("cluster %s or user %s not found", id, username)
	case http.StatusForbidden:
		return response, fmt.Errorf("access denied: you don't have permission to get credentials for user %s in cluster %s", username, id)
	default:
		return response, fmt.Errorf("failed to get user credentials (status %d): %s", res.StatusCode, string(body))
	}
}

func (c *Client) ListUsers(ctx context.Context, id string) (ListUsersResponse, error) {
	var response ListUsersResponse

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgresv2/%s/users", c.BaseURL(), id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.HTTPClient().Do(req)
	if err != nil {
		return response, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return response, fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusOK:
		if err = json.Unmarshal(body, &response); err != nil {
			return response, fmt.Errorf("failed to decode response: %w", err)
		}

		return response, nil
	case http.StatusNotFound:
		return response, fmt.Errorf("cluster %s not found", id)
	case http.StatusForbidden:
		return response, fmt.Errorf("access denied: you don't have permission to list users for cluster %s", id)
	default:
		return response, fmt.Errorf("failed to list users (status %d): %s", res.StatusCode, string(body))
	}
}

func (c *Client) CreateCluster(ctx context.Context, input CreateClusterInput) (CreateClusterResponse, error) {
	var response CreateClusterResponse

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/organizations/%s/postgresv2", c.BaseURL(), input.OrgSlug)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(input); err != nil {
		return response, fmt.Errorf("failed to encode request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.HTTPClient().Do(req)
	if err != nil {
		return response, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return response, fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusCreated:
		if err = json.Unmarshal(body, &response); err != nil {
			return response, fmt.Errorf("failed to decode response: %w", err)
		}

		return response, nil
	case http.StatusNotFound:
		return response, fmt.Errorf("organization %s not found", input.OrgSlug)
	case http.StatusForbidden:
		if err = json.Unmarshal(body, &response); err == nil {
			if response.Errors.Detail != "" {
				return response, errors.New(response.Errors.Detail)
			}
		}

		return response, fmt.Errorf("failed to create cluster (status %d): %s", res.StatusCode, string(body))
	case http.StatusInternalServerError:
		return response, fmt.Errorf("server error: %s", string(body))
	default:
		return response, fmt.Errorf("failed to create cluster (status %d): %s", res.StatusCode, string(body))
	}
}

func (c *Client) DestroyCluster(ctx context.Context, orgSlug string, id string) error {
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/organizations/%s/postgresv2/%s", c.BaseURL(), orgSlug, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.HTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusAccepted:
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("cluster %s not found", id)
	case http.StatusForbidden:
		return fmt.Errorf("access denied: you don't have permission to destroy cluster %s", id)
	default:
		return fmt.Errorf("failed to destroy cluster (status %d): %s", res.StatusCode, string(body))
	}
}

func (c *Client) ListClusterBackups(ctx context.Context, clusterID string) (ListClusterBackupsResponse, error) {
	var response ListClusterBackupsResponse

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgresv2/%s/backups", c.BaseURL(), clusterID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.HTTPClient().Do(req)
	if err != nil {
		return response, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return response, fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusOK:
		if err = json.Unmarshal(body, &response); err != nil {
			return response, fmt.Errorf("failed to decode response: %w", err)
		}

		return response, nil
	case http.StatusNotFound:
		return response, fmt.Errorf("cluster %s not found", clusterID)
	case http.StatusForbidden:
		return response, fmt.Errorf("access denied: you don't have permission to list backups for cluster %s", clusterID)
	default:
		return response, fmt.Errorf("failed to list backups (status %d): %s", res.StatusCode, string(body))
	}
}

func (c *Client) CreateClusterBackup(ctx context.Context, clusterID string, input CreateClusterBackupInput) error {
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgresv2/%s/backups", c.BaseURL(), clusterID)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(input); err != nil {
		return fmt.Errorf("failed to encode request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.HTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted:
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("cluster %s not found", clusterID)
	case http.StatusForbidden:
		return fmt.Errorf("access denied: you don't have permission to create backups for cluster %s", clusterID)
	default:
		return fmt.Errorf("failed to create backup (status %d): %s", res.StatusCode, string(body))
	}
}

func (c *Client) RestoreClusterBackup(ctx context.Context, clusterID string, input RestoreClusterBackupInput) (RestoreClusterBackupResponse, error) {
	var response RestoreClusterBackupResponse

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgresv2/%s/restore", c.BaseURL(), clusterID)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(input); err != nil {
		return response, fmt.Errorf("failed to encode request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.HTTPClient().Do(req)
	if err != nil {
		return response, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return response, fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusOK, http.StatusCreated:
		if err = json.Unmarshal(body, &response); err != nil {
			return response, fmt.Errorf("failed to decode response: %w", err)
		}

		return response, nil
	case http.StatusNotFound:
		return response, fmt.Errorf("cluster %s not found", clusterID)
	case http.StatusForbidden:
		return response, fmt.Errorf("access denied: you don't have permission to restore cluster %s", clusterID)
	default:
		return response, fmt.Errorf("failed to restore backup (status %d): %s", res.StatusCode, string(body))
	}
}

func (c *Client) CreateAttachment(ctx context.Context, clusterId string, input CreateAttachmentInput) (CreateAttachmentResponse, error) {
	var response CreateAttachmentResponse

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgresv2/%s/attachments", c.BaseURL(), clusterId)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(input); err != nil {
		return response, fmt.Errorf("failed to encode request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.HTTPClient().Do(req)
	if err != nil {
		return response, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return response, fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusOK, http.StatusCreated:
		if err = json.Unmarshal(body, &response); err != nil {
			return response, fmt.Errorf("failed to decode response: %w", err)
		}

		return response, nil
	case http.StatusNotFound:
		return response, fmt.Errorf("cluster %s not found", clusterId)
	case http.StatusForbidden:
		return response, fmt.Errorf("access denied: you don't have permission to attach cluster %s", clusterId)
	default:
		return response, fmt.Errorf("failed to create attachment (status %d): %s", res.StatusCode, string(body))
	}
}

func (c *Client) DeleteAttachment(ctx context.Context, clusterId string, appName string) (DeleteAttachmentResponse, error) {
	var response DeleteAttachmentResponse

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgresv2/%s/attachments/%s", c.BaseURL(), clusterId, appName)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())

	res, err := c.HTTPClient().Do(req)
	if err != nil {
		return response, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return response, fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusOK:
		if err = json.Unmarshal(body, &response); err != nil {
			return response, fmt.Errorf("failed to decode response: %w", err)
		}

		return response, nil
	case http.StatusNotFound:
		return response, fmt.Errorf("attachment not found for app '%s' on cluster %s", appName, clusterId)
	case http.StatusForbidden:
		return response, fmt.Errorf("access denied: you don't have permission to detach from cluster %s", clusterId)
	default:
		return response, fmt.Errorf("failed to delete attachment (status %d): %s", res.StatusCode, string(body))
	}
}
