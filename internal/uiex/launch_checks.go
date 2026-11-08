package uiex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"

	"github.com/superfly/flyctl/internal/config"
)

type LaunchChecks struct {
	CanLaunch bool `json:"can_launch"`
	Checks    struct {
		MachineQuota MachineQuotaCheck `json:"machine_quota"`
	} `json:"checks"`
}

// MachineQuotaCheck uses the same Machine count flaps enforces. A nil Limit
// means the org has no limit.
type MachineQuotaCheck struct {
	Passed  bool `json:"passed"`
	Current int  `json:"current"`
	Limit   *int `json:"limit"`
}

func (c *Client) GetLaunchChecks(ctx context.Context, orgSlug string) (*LaunchChecks, error) {
	var response struct {
		Data LaunchChecks `json:"data"`
	}

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/organizations/%s/launch-checks", c.baseUrl, neturl.PathEscape(orgSlug))

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
		if err = json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("failed to decode response, please try again: %w", err)
		}

		return &response.Data, nil
	default:
		return nil, &StatusError{Op: "get launch checks for " + orgSlug, StatusCode: res.StatusCode, Body: string(body)}
	}
}
