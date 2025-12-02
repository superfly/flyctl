package uiex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/superfly/flyctl/internal/config"
)

type DeploymentStrategy string

const (
	// Launch all new instances before shutting down previous instances
	DeploymentStrategyBluegreen DeploymentStrategy = "BLUEGREEN"
	// Ensure new instances are healthy before continuing with a rolling deployment
	DeploymentStrategyCanary DeploymentStrategy = "CANARY"
	// Deploy new instances all at once
	DeploymentStrategyImmediate DeploymentStrategy = "IMMEDIATE"
	// Incrementally replace old instances with new ones
	DeploymentStrategyRolling DeploymentStrategy = "ROLLING"
	// Incrementally replace old instances with new ones, 1 by 1
	DeploymentStrategyRollingOne DeploymentStrategy = "ROLLING_ONE"
	// Deploy new instances all at once
	DeploymentStrategySimple DeploymentStrategy = "SIMPLE"
)

type Release struct {
	ID                 string             `json:"id"`
	Version            int                `json:"version"`
	Stable             bool               `json:"stable"`
	InProgress         bool               `json:"in_progress"`
	Status             string             `json:"status"`
	DeploymentStrategy DeploymentStrategy `json:"strategy"`
	User               string             `json:"user"`
	CreatedAt          time.Time          `json:"created_at"`
	ImageRef           string             `json:"image_ref"`
}

type CreateReleaseRequest struct {
	AppName    string             `json:"app_name"`
	BuildId    int64              `json:"build_id"`
	Definition any                `json:"definition"`
	Image      string             `json:"image"`
	Strategy   DeploymentStrategy `json:"strategy"`
}

func (c *Client) GetAllAppsCurrentReleaseTimestamps(ctx context.Context) (out *map[string]time.Time, err error) {
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/releases/all_current", c.baseUrl)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusOK:
		if err = json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("failed to decode response, please try again: %w", err)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("failed to get current release timestamps (status %d): %s", res.StatusCode, string(body))
	}
}

func (c *Client) ListReleases(ctx context.Context, appName string, limit int) ([]Release, error) {
	var err error

	var response struct {
		Releases []Release `json:"releases"`
	}

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/apps/%s/releases", c.baseUrl, appName)

	if limit > 0 {
		url = fmt.Sprintf("%s?limit=%d", url, limit)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return []Release{}, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return []Release{}, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return []Release{}, fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusOK:
		if err = json.Unmarshal(body, &response); err != nil {
			return []Release{}, fmt.Errorf("failed to decode response, please try again: %w", err)
		}
		return response.Releases, nil
	default:
		return []Release{}, fmt.Errorf("failed to list releases (status %d): %s", res.StatusCode, string(body))
	}
}

// GetCurrentRelease retrieves the current release for an app.
// Returns nil release (without error) if the app has no current release (404).
func (c *Client) GetCurrentRelease(ctx context.Context, appName string) (release *Release, err error) {
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/apps/%s/releases/current", c.baseUrl, appName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusOK:
		if err = json.Unmarshal(body, &release); err != nil {
			return nil, fmt.Errorf("failed to decode response, please try again: %w", err)
		}
		return release, nil
	case http.StatusNotFound:
		return nil, nil
	default:
		return nil, fmt.Errorf("failed to get current release (status %d): %s", res.StatusCode, string(body))
	}
}

func (c *Client) CreateRelease(ctx context.Context, request CreateReleaseRequest) (release *Release, err error) {
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/releases", c.baseUrl)

	var response struct {
		Release Release `json:"release"`
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(request); err != nil {
		return nil, fmt.Errorf("failed to encode request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusOK, http.StatusCreated:
		if err = json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		return &response.Release, nil
	default:
		return nil, fmt.Errorf("failed to create release (status %d): %s", res.StatusCode, string(body))
	}
}

func (c *Client) UpdateRelease(ctx context.Context, releaseID, status string, metadata any) (response *Release, err error) {
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/releases/%s", c.baseUrl, releaseID)

	request := map[string]any{
		"status":   status,
		"metadata": metadata,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(request); err != nil {
		return nil, fmt.Errorf("failed to encode request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusOK:
		if err = json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		return response, nil
	default:
		return nil, fmt.Errorf("failed to update release (status %d): %s", res.StatusCode, string(body))
	}
}
