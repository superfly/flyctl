package v2

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/uiex"
)

type Client struct {
	uiex.Client
}

func (c *Client) ListManagedClusters(ctx context.Context, orgSlug string, deleted bool) (ListManagedClustersResponse, error) {
	var response ListManagedClustersResponse

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/organizations/%s/postgresv2", c.BaseUrl, orgSlug)
	if deleted {
		url = fmt.Sprintf("%s/api/v1/organizations/%s/postgresv2/deleted", c.BaseUrl, orgSlug)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.HttpClient.Do(req)
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
