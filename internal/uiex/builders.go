package uiex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/superfly/flyctl/internal/config"
)

type CreateBuildRequest struct {
	AppName             string   `json:"app_name,omitempty"`
	BuilderType         string   `json:"builder_type,omitempty"`
	MachineId           string   `json:"machine_id,omitempty"`
	StrategiesAvailable []string `json:"strategies_available,omitempty"`
}

type FinishBuildRequest struct {
	BuildId             int64                  `json:"build_id"`
	AppName             string                 `json:"app_name"`
	MachineId           string                 `json:"machine_id"`
	Status              string                 `json:"status"`
	StrategiesAttempted []BuildStrategyAttempt `json:"strategies_attempted"`
	BuilderMeta         BuilderMeta            `json:"builder_meta"`
	FinalImage          BuildFinalImage        `json:"final_image"`
	Timings             BuildTimings           `json:"timings"`
	Logs                string                 `json:"logs"`
}

type BuildStrategyAttempt struct {
	Error    string `json:"error"`
	Note     string `json:"note"`
	Result   string `json:"result"`
	Strategy string `json:"strategy"`
}

type BuilderMeta struct {
	BuilderType     string `json:"builder_type"`
	BuildkitEnabled bool   `json:"buildkit_enabled"`
	DockerVersion   string `json:"docker_version"`
	Platform        string `json:"platform"`
	RemoteAppName   string `json:"remote_app_name"`
	RemoteMachineId string `json:"remote_machine_id"`
}

type BuildFinalImage struct {
	Id        string `json:"id"`
	SizeBytes int64  `json:"size_bytes"`
	Tag       string `json:"tag"`
}

type BuildTimings struct {
	BuildAndPushMs int64 `json:"build_and_push_ms"`
	BuildMs        int64 `json:"build_ms"`
	BuilderInitMs  int64 `json:"builder_init_ms"`
	ContextBuildMs int64 `json:"context_build_ms"`
	ImageBuildMs   int64 `json:"image_build_ms"`
	PushMs         int64 `json:"push_ms"`
}

type BuildResponse struct {
	Id              int64  `json:"id"`
	Status          string `json:"status"`
	WallclockTimeMs int    `json:"wallclock_time_ms"`
}

func (c *Client) CreateBuild(ctx context.Context, in CreateBuildRequest) (*BuildResponse, error) {
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/builds", c.baseUrl)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(in); err != nil {
		return nil, err
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

	var response BuildResponse

	switch res.StatusCode {
	case http.StatusOK:
		if err = json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("failed to decode response, please try again: %w", err)
		}

		return &response, nil

	default:
		return nil, fmt.Errorf("ensure depot builder failed, please try again (status %d): %s", res.StatusCode, string(body))
	}
}

func (c *Client) FinishBuild(ctx context.Context, in FinishBuildRequest) (*BuildResponse, error) {
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/builds/finish", c.baseUrl)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(in); err != nil {
		return nil, err
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

	var response BuildResponse

	switch res.StatusCode {
	case http.StatusOK:
		if err = json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("failed to decode response, please try again: %w", err)
		}

		return &response, nil

	default:
		return nil, fmt.Errorf("ensure depot builder failed, please try again (status %d): %s", res.StatusCode, string(body))
	}
}

type EnsureDepotBuilderRequest struct {
	AppName        *string `json:"app_name,omitempty"`
	BuilderScope   *string `json:"builder_scope,omitempty"`
	OrganizationId *string `json:"organization_id,omitempty"`
	Region         *string `json:"region,omitempty"`
}

type EnsureDepotBuilderResponse struct {
	BuildId    *string `json:"build_id"`
	BuildToken *string `json:"build_token"`
}

func (c *Client) EnsureDepotBuilder(ctx context.Context, in EnsureDepotBuilderRequest) (*EnsureDepotBuilderResponse, error) {
	var response EnsureDepotBuilderResponse
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/builds/depot_builder", c.baseUrl)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(in); err != nil {
		return nil, err
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
	case http.StatusOK:
		if err = json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("failed to decode response, please try again: %w", err)
		}

		return &response, nil

	default:
		return nil, fmt.Errorf("ensure depot builder failed, please try again (status %d): %s", res.StatusCode, string(body))
	}
}

type CreateFlyManagedBuilderParams struct {
	Region string `json:"region"`
}
type CreateFlyManagedBuilderInput struct {
	Builder CreateFlyManagedBuilderParams `json:"builder"`
}

type FlyManagedBuilder struct {
	AppName   string `json:"app_name"`
	MachineID string `json:"machine_id"`
}

type CreateFlyManagedBuilderResponse struct {
	Data   FlyManagedBuilder `json:"data"`
	Errors DetailedErrors    `json:"errors"`
}

func (c *Client) CreateFlyManagedBuilder(ctx context.Context, orgSlug string, region string) (CreateFlyManagedBuilderResponse, error) {
	var response CreateFlyManagedBuilderResponse
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/organizations/%s/builders", c.baseUrl, orgSlug)

	input := &CreateFlyManagedBuilderInput{
		Builder: CreateFlyManagedBuilderParams{
			Region: region,
		},
	}
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
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return response, fmt.Errorf("failed to read response body: %w", err)
	}

	switch res.StatusCode {
	case http.StatusCreated:
		if err = json.Unmarshal(body, &response); err != nil {
			return response, fmt.Errorf("failed to decode response, please try again: %w", err)
		}

		return response, nil

	default:
		return response, fmt.Errorf("builder creation failed, please try again (status %d): %s", res.StatusCode, string(body))
	}
}
