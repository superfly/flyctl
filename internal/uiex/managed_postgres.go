package uiex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/config"
)

type ManagedClusterIpAssignments struct {
	Direct string `json:"direct"`
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

type AttachedApp struct {
	Name string `json:"name"`
	Id   int64  `json:"id"`
}

type ManagedCluster struct {
	Id            string                      `json:"id"`
	Name          string                      `json:"name"`
	Region        string                      `json:"region"`
	Status        string                      `json:"status"`
	Plan          string                      `json:"plan"`
	Disk          int                         `json:"disk"`
	Replicas      int                         `json:"replicas"`
	Organization  fly.Organization            `json:"organization"`
	IpAssignments ManagedClusterIpAssignments `json:"ip_assignments"`
	AttachedApps  []AttachedApp               `json:"attached_apps"`
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

func (c *Client) ListManagedClusters(ctx context.Context, orgSlug string, deleted bool) (ListManagedClustersResponse, error) {
	var response ListManagedClustersResponse

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/organizations/%s/postgres", c.baseUrl, orgSlug)
	if deleted {
		url = fmt.Sprintf("%s/api/v1/organizations/%s/postgres/deleted", c.baseUrl, orgSlug)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
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
		return response, fmt.Errorf("organization %s not found", orgSlug)
	default:
		return response, fmt.Errorf("failed to list clusters (status %d): %s", res.StatusCode, string(body))
	}

}

func (c *Client) GetManagedCluster(ctx context.Context, orgSlug string, id string) (GetManagedClusterResponse, error) {
	var response GetManagedClusterResponse
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/organizations/%s/postgres/%s", c.baseUrl, orgSlug, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
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

func (c *Client) GetManagedClusterById(ctx context.Context, id string) (GetManagedClusterResponse, error) {
	var response GetManagedClusterResponse
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgres/%s", c.baseUrl, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return response, err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		if err = json.NewDecoder(res.Body).Decode(&response); err != nil {
			return response, fmt.Errorf("failed to decode response, please try again: %w", err)
		}

		return response, nil
	case http.StatusNotFound:
		return response, fmt.Errorf("Cluster %s not found", id)
	default:
		return response, fmt.Errorf("Something went wrong")
	}
}

type CreateUserInput struct {
	DbName   string `json:"db_name"`
	UserName string `json:"user_name"`
}

type DetailedErrors struct {
	Detail string `json:"detail"`
}

type CreateUserResponse struct {
	ConnectionUri string         `json:"connection_uri"`
	Ok            bool           `json:"ok"`
	Errors        DetailedErrors `json:"errors"`
}

func (c *Client) CreateUser(ctx context.Context, id string, input CreateUserInput) (CreateUserResponse, error) {
	var response CreateUserResponse
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgres/%s/users", c.baseUrl, id)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(input); err != nil {
		return response, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return response, err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusCreated:
		if err = json.NewDecoder(res.Body).Decode(&response); err != nil {
			return response, fmt.Errorf("failed to decode response, please try again: %w", err)
		}

		if !response.Ok {
			if response.Errors.Detail != "" {
				return response, fmt.Errorf("Failed to create user with error: %s", response.Errors.Detail)
			} else {
				return response, fmt.Errorf("Something went wrong creating user. Please try again")
			}
		}

		return response, nil

	default:
		if err = json.NewDecoder(res.Body).Decode(&response); err != nil {
			return response, fmt.Errorf("failed to decode response, please try again: %w", err)
		}

		if response.Errors.Detail != "" {
			return response, fmt.Errorf("Failed to create user with error: %s", response.Errors.Detail)
		}

		return response, fmt.Errorf("Failed to create user with error: %s", response.Errors.Detail)
	}
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

func (c *Client) CreateUserWithRole(ctx context.Context, id string, input CreateUserWithRoleInput) (CreateUserWithRoleResponse, error) {
	var response CreateUserWithRoleResponse
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgres/%s/users", c.baseUrl, id)

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

	res, err := c.httpClient.Do(req)
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

type UpdateUserRoleInput struct {
	Role string `json:"role"` // 'schema_admin' | 'writer' | 'reader'
}

type UpdateUserRoleResponse struct {
	Data User `json:"data"`
}

func (c *Client) UpdateUserRole(ctx context.Context, id string, username string, input UpdateUserRoleInput) (UpdateUserRoleResponse, error) {
	var response UpdateUserRoleResponse
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgres/%s/users/%s", c.baseUrl, id, username)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(input); err != nil {
		return response, fmt.Errorf("failed to encode request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, &buf)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
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
		return response, fmt.Errorf("access denied: you don't have permission to update users for cluster %s", id)
	default:
		return response, fmt.Errorf("failed to update user role (status %d): %s", res.StatusCode, string(body))
	}
}

func (c *Client) DeleteUser(ctx context.Context, id string, username string) error {
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgres/%s/users/%s", c.baseUrl, id, username)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
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
	url := fmt.Sprintf("%s/api/v1/postgres/%s/users/%s/credentials", c.baseUrl, id, username)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
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
	url := fmt.Sprintf("%s/api/v1/postgres/%s/users", c.baseUrl, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
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

func (c *Client) ListDatabases(ctx context.Context, id string) (ListDatabasesResponse, error) {
	var response ListDatabasesResponse
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgres/%s/databases", c.baseUrl, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
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

func (c *Client) CreateDatabase(ctx context.Context, id string, input CreateDatabaseInput) (CreateDatabaseResponse, error) {
	var response CreateDatabaseResponse
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgres/%s/databases", c.baseUrl, id)

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

	res, err := c.httpClient.Do(req)
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
		return response, fmt.Errorf("access denied: you don't have permission to create databases for cluster %s", id)
	default:
		return response, fmt.Errorf("failed to create database (status %d): %s", res.StatusCode, string(body))
	}
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
	Ok     bool           `json:"ok"`
	Errors DetailedErrors `json:"errors"`
	Data   struct {
		Id             string                      `json:"id"`
		Name           string                      `json:"name"`
		Status         *string                     `json:"status"`
		Plan           string                      `json:"plan"`
		Environment    *string                     `json:"environment"`
		Region         string                      `json:"region"`
		Organization   fly.Organization            `json:"organization"`
		Replicas       int                         `json:"replicas"`
		Disk           int                         `json:"disk"`
		IpAssignments  ManagedClusterIpAssignments `json:"ip_assignments"`
		PostGISEnabled bool                        `json:"postgis_enabled"`
	} `json:"data"`
}

func (c *Client) CreateCluster(ctx context.Context, input CreateClusterInput) (CreateClusterResponse, error) {
	var response CreateClusterResponse
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/organizations/%s/postgres", c.baseUrl, input.OrgSlug)

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

	res, err := c.httpClient.Do(req)
	if err != nil {
		return response, err
	}
	defer res.Body.Close()

	// Read the response body to get error details
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

// ListMPGRegions returns the list of regions available for Managed Postgres
// TODO: Implement the actual API endpoint on the backend
func (c *Client) ListMPGRegions(ctx context.Context, orgSlug string) (ListMPGRegionsResponse, error) {
	var response ListMPGRegionsResponse
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/organizations/%s/postgres/regions", c.baseUrl, orgSlug)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
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

// ListManagedClusterBackups returns the list of backups for a managed Postgres cluster
func (c *Client) ListManagedClusterBackups(ctx context.Context, clusterID string) (ListManagedClusterBackupsResponse, error) {
	var response ListManagedClusterBackupsResponse
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgres/%s/backups", c.baseUrl, clusterID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
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

// CreateManagedClusterBackup creates a new backup for a managed Postgres cluster
func (c *Client) CreateManagedClusterBackup(ctx context.Context, clusterID string, input CreateManagedClusterBackupInput) (CreateManagedClusterBackupResponse, error) {
	var response CreateManagedClusterBackupResponse
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgres/%s/backups", c.baseUrl, clusterID)

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

	res, err := c.httpClient.Do(req)
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
		return response, fmt.Errorf("access denied: you don't have permission to create backups for cluster %s", clusterID)
	default:
		return response, fmt.Errorf("failed to create backup (status %d): %s", res.StatusCode, string(body))
	}
}

// RestoreManagedClusterBackup restores a managed Postgres cluster from a backup
func (c *Client) RestoreManagedClusterBackup(ctx context.Context, clusterID string, input RestoreManagedClusterBackupInput) (RestoreManagedClusterBackupResponse, error) {
	var response RestoreManagedClusterBackupResponse
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgres/%s/restore", c.baseUrl, clusterID)

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

	res, err := c.httpClient.Do(req)
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

// DestroyCluster permanently destroys a managed Postgres cluster
func (c *Client) DestroyCluster(ctx context.Context, orgSlug string, id string) error {
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/organizations/%s/postgres/%s", c.baseUrl, orgSlug, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
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

// CreateAttachment creates a ManagedServiceAttachment record linking an app to a managed Postgres cluster
func (c *Client) CreateAttachment(ctx context.Context, clusterId string, input CreateAttachmentInput) (CreateAttachmentResponse, error) {
	var response CreateAttachmentResponse
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgres/%s/attachments", c.baseUrl, clusterId)

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

	res, err := c.httpClient.Do(req)
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
