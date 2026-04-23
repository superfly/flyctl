package v2

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/uiex"
)

type contextKey struct{}

var clientContextKey = &contextKey{}

type ClientV2 interface {
	// List the available regions for MPG V2
	ListRegions(ctx context.Context, orgSlug string) (ListRegionsResponse, error)
	ListManagedClusters(ctx context.Context, orgSlug string, deleted bool) (ListManagedClustersResponse, error)
	// GetManagedCluster(ctx context.Context, orgSlug string, id string) (GetManagedClusterResponse, error)
	// GetManagedClusterById(ctx context.Context, id string) (GetManagedClusterResponse, error)
	// CreateUser(ctx context.Context, id string, input CreateUserInput) (CreateUserResponse, error)
	// CreateUserWithRole(ctx context.Context, id string, input CreateUserWithRoleInput) (CreateUserWithRoleResponse, error)
	// UpdateUserRole(ctx context.Context, id string, username string, input UpdateUserRoleInput) (UpdateUserRoleResponse, error)
	// DeleteUser(ctx context.Context, id string, username string) error
	// GetUserCredentials(ctx context.Context, id string, username string) (GetUserCredentialsResponse, error)
	// ListUsers(ctx context.Context, id string) (ListUsersResponse, error)
	ListDatabases(ctx context.Context, id string) (ListDatabasesResponse, error)
	CreateDatabase(ctx context.Context, id string, input CreateDatabaseInput) (CreateDatabaseResponse, error)
	// CreateCluster(ctx context.Context, input CreateClusterInput) (CreateClusterResponse, error)
	// DestroyCluster(ctx context.Context, orgSlug string, id string) error
	// ListManagedClusterBackups(ctx context.Context, clusterID string) (ListManagedClusterBackupsResponse, error)
	// CreateManagedClusterBackup(ctx context.Context, clusterID string, input CreateManagedClusterBackupInput) (CreateManagedClusterBackupResponse, error)
	// RestoreManagedClusterBackup(ctx context.Context, clusterID string, input RestoreManagedClusterBackupInput) (RestoreManagedClusterBackupResponse, error)
	// CreateAttachment(ctx context.Context, clusterId string, input CreateAttachmentInput) (CreateAttachmentResponse, error)
	// DeleteAttachment(ctx context.Context, clusterId string, appName string) (DeleteAttachmentResponse, error)
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

type ListManagedClustersResponse struct {
	Data []ManagedCluster `json:"data"`
}

type ManagedCluster struct {
	Id            string            `json:"id"`
	Name          string            `json:"name"`
	Status        string            `json:"status"`
	Engine        string            `json:"engine"`
	Plan          string            `json:"plan"`
	Region        string            `json:"region"`
	Context       string            `json:"context"`
	Organization  *fly.Organization `json:"organization"`
	CreatedAt     time.Time         `json:"created_at"`
	MpgdClusterId string            `json:"mpgd_cluster_id"`
	DeletedAt     *time.Time        `json:"deleted_at,omitempty"`
}

func (c *Client) ListManagedClusters(ctx context.Context, orgSlug string, deleted bool) (ListManagedClustersResponse, error) {
	var response ListManagedClustersResponse

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/organizations/%s/postgresv2", c.BaseURL(), orgSlug)
	if deleted {
		url = fmt.Sprintf("%s/api/v1/organizations/%s/postgresv2/deleted", c.BaseURL(), orgSlug)
	}

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
		return response, fmt.Errorf("organization %s not found", orgSlug)
	default:
		return response, fmt.Errorf("failed to list clusters (status %d): %s", res.StatusCode, string(body))
	}
}

type Region struct {
	Code      string `json:"code"`      // e.g., "fra"
	Available bool   `json:"available"` // Whether this region supports MPG
}

type ListRegionsResponse struct {
	Data []Region `json:"data"`
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

type ListDatabasesResponse struct {
	Data []Database `json:"data"`
}

type Database struct {
	Name string `json:"name"`
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

func (c *Client) CreateDatabase(ctx context.Context, id string, input CreateDatabaseInput) (CreateDatabaseResponse, error) {
	var response CreateDatabaseResponse

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/postgresv2/%s/databases", c.BaseURL(), id)

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
		return response, fmt.Errorf("access denied: you don't have permission to create databases for cluster %s", id)
	default:
		return response, fmt.Errorf("failed to create database (status %d): %s", res.StatusCode, string(body))
	}
}

// type CreateClusterInput struct {
// 	Name           string `json:"name"`
// 	Region         string `json:"region"`
// 	Plan           string `json:"plan"`
// 	OrgSlug        string `json:"org_slug"`
// 	Disk           int    `json:"disk"`
// 	PostGISEnabled bool   `json:"postgis_enabled"`
// 	PGMajorVersion string `json:"pg_major_version"`
// }

// type CreateClusterResponse struct {
// 	Ok     bool                `json:"ok"`
// 	Errors uiex.DetailedErrors `json:"errors"`
// 	Data   struct {
// 		Id             string                          `json:"id"`
// 		Name           string                          `json:"name"`
// 		Status         *string                         `json:"status"`
// 		Plan           string                          `json:"plan"`
// 		Environment    *string                         `json:"environment"`
// 		Region         string                          `json:"region"`
// 		Organization   fly.Organization                `json:"organization"`
// 		Replicas       int                             `json:"replicas"`
// 		Disk           int                             `json:"disk"`
// 		IpAssignments  mpg.ManagedClusterIpAssignments `json:"ip_assignments"`
// 		PostGISEnabled bool                            `json:"postgis_enabled"`
// 	} `json:"data"`
// }

// func (c *Client) CreateCluster(ctx context.Context, input CreateClusterInput) (CreateClusterResponse, error) {
// 	var response CreateClusterResponse

// 	cfg := config.FromContext(ctx)
// 	url := fmt.Sprintf("%s/api/v1/organizations/%s/postgres", c.BaseURL(), input.OrgSlug)

// 	var buf bytes.Buffer
// 	if err := json.NewEncoder(&buf).Encode(input); err != nil {
// 		return response, fmt.Errorf("failed to encode request body: %w", err)
// 	}

// 	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
// 	if err != nil {
// 		return response, fmt.Errorf("failed to create request: %w", err)
// 	}

// 	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
// 	req.Header.Add("Content-Type", "application/json")

// 	res, err := c.HTTPClient().Do(req)
// 	if err != nil {
// 		return response, err
// 	}
// 	defer res.Body.Close()

// 	// Read the response body to get error details
// 	body, err := io.ReadAll(res.Body)
// 	if err != nil {
// 		return response, fmt.Errorf("failed to read response body: %w", err)
// 	}

// 	switch res.StatusCode {
// 	case http.StatusCreated:
// 		if err = json.Unmarshal(body, &response); err != nil {
// 			return response, fmt.Errorf("failed to decode response: %w", err)
// 		}

// 		return response, nil
// 	case http.StatusNotFound:
// 		return response, fmt.Errorf("organization %s not found", input.OrgSlug)
// 	case http.StatusForbidden:
// 		if err = json.Unmarshal(body, &response); err == nil {
// 			if response.Errors.Detail != "" {
// 				return response, errors.New(response.Errors.Detail)
// 			}
// 		}

// 		return response, fmt.Errorf("failed to create cluster (status %d): %s", res.StatusCode, string(body))
// 	case http.StatusInternalServerError:
// 		return response, fmt.Errorf("server error: %s", string(body))
// 	default:
// 		return response, fmt.Errorf("failed to create cluster (status %d): %s", res.StatusCode, string(body))
// 	}
// }
