package uiex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/superfly/flyctl/lib/config"
)

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
