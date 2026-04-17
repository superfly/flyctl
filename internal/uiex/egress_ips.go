package uiex

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/superfly/flyctl/internal/config"
)

func (c *Client) PromoteMachineEgressIP(ctx context.Context, appName string, egressIP string) error {
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/apps/%s/egress_ips/%s/promote", c.baseUrl, appName, egressIP)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
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
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	default:
		return fmt.Errorf("failed to promote egress IP (status %d): %s", res.StatusCode, string(body))
	}
}
