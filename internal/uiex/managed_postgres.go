package uiex

import (
	"bytes"
	"context"
	"encoding/json"
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

type GetManagedClusterResponse struct {
	Data        ManagedCluster                       `json:"data"`
	Credentials GetManagedClusterCredentialsResponse `json:"credentials"`
}

func (c *Client) ListManagedClusters(ctx context.Context, orgSlug string) (ListManagedClustersResponse, error) {
	var response ListManagedClustersResponse

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/organizations/%s/postgres", c.baseUrl, orgSlug)

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
		return response, err
	default:
		return response, err
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

	switch res.StatusCode {
	case http.StatusOK:
		if err = json.NewDecoder(res.Body).Decode(&response); err != nil {
			return response, fmt.Errorf("failed to decode response, please try again: %w", err)
		}
		return response, nil
	case http.StatusNotFound:
		return response, err
	default:
		return response, err
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

type CreateClusterInput struct {
	Name           string `json:"name"`
	Region         string `json:"region"`
	Plan           string `json:"plan"`
	OrgSlug        string `json:"org_slug"`
	Disk           int    `json:"disk"`
	PostGISEnabled bool   `json:"postgis_enabled"`
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
				return response, fmt.Errorf(response.Errors.Detail)
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
	case http.StatusOK, http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("cluster %s not found", id)
	case http.StatusForbidden:
		return fmt.Errorf("access denied: you don't have permission to destroy cluster %s", id)
	default:
		return fmt.Errorf("failed to destroy cluster (status %d): %s", res.StatusCode, string(body))
	}
}
