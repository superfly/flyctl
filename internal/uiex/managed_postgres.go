package uiex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/config"
)

type ManagedClusterIpAssignments struct {
	Direct string `json:"direct"`
}

type ManagedCluster struct {
	Id            string                      `json:"id"`
	Name          string                      `json:"name"`
	Region        string                      `json:"region"`
	Status        string                      `json:"status"`
	Plan          string                      `json:"plan"`
	Organization  fly.Organization            `json:"organization"`
	IpAssignments ManagedClusterIpAssignments `json:"ip_assignments"`
}

type ListManagedClustersResponse struct {
	Data []ManagedCluster `json:"data"`
}

type GetManagedClusterPasswordResponse struct {
	Status string `json:"status"`
	Value  string `json:"value"`
}

type GetManagedClusterResponse struct {
	Data     ManagedCluster                    `json:"data"`
	Password GetManagedClusterPasswordResponse `json:"password"`
}

type CreateManagedClusterRequest struct {
	Name        string `json:"name"`
	Region      string `json:"region"`
	Password    string `json:"password,omitempty"`
	VolumeSize  int    `json:"volume_size"`
	ClusterSize int    `json:"cluster_size"`
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

func (c *Client) CreateManagedCluster(ctx context.Context, orgSlug string, input *CreateManagedClusterRequest) (GetManagedClusterResponse, error) {
	var response GetManagedClusterResponse

	jsonBody, err := json.Marshal(input)
	if err != nil {
		return response, fmt.Errorf("failed to marshal request: %w", err)
	}

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/organizations/%s/postgres", c.baseUrl, orgSlug)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return response, fmt.Errorf("failed to send request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		return response, fmt.Errorf("failed to create cluster: unexpected status code %d", res.StatusCode)
	}

	if err = json.NewDecoder(res.Body).Decode(&response); err != nil {
		return response, fmt.Errorf("failed to decode response: %w", err)
	}

	return response, nil
}
