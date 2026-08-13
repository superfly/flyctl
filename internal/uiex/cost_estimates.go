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

type CostEstimateRequest struct {
	SchemaVersion int                  `json:"schema_version"`
	Operation     string               `json:"operation"`
	Currency      string               `json:"currency,omitempty"`
	Changes       []CostEstimateChange `json:"changes"`
	Client        *CostEstimateClient  `json:"client,omitempty"`
}

type CostEstimateClient struct {
	Name          string `json:"name"`
	Version       string `json:"version,omitempty"`
	SourceCommand string `json:"source_command,omitempty"`
}

type CostEstimateChange struct {
	Kind    string         `json:"kind"`
	Action  string         `json:"action"`
	Ref     string         `json:"ref,omitempty"`
	Count   int            `json:"count,omitempty"`
	Current any            `json:"current,omitempty"`
	Desired any            `json:"desired,omitempty"`
	Source  any            `json:"source,omitempty"`
	Usage   map[string]any `json:"usage,omitempty"`
}

type CostEstimateResponse struct {
	Data json.RawMessage `json:"data"`
}

func (c *Client) CreateCostEstimate(ctx context.Context, orgSlug string, in CostEstimateRequest) (*CostEstimateResponse, error) {
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/organizations/%s/cost-estimates", c.baseUrl, orgSlug)

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
	req.Header.Add("Accept", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to create cost estimate (status %d): %s", res.StatusCode, string(body))
	}

	var response CostEstimateResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to decode response, please try again: %w", err)
	}

	return &response, nil
}
