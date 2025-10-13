package uiex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/superfly/flyctl/internal/config"
	"github.com/tmaxmax/go-sse"
)

type RemoteDeploymentStrategy string

const (
	RemoteDeploymentStrategyRolling RemoteDeploymentStrategy = "rolling"
)

type RemoteDeploymentRequest struct {
	Organization string                   `json:"organization"`
	Config       any                      `json:"config"`
	Image        string                   `json:"image"`
	Strategy     RemoteDeploymentStrategy `json:"strategy"`
	BuildId      string                   `json:"build_id"`
}

// CreateDeploy creates a new remote deploy for the given app and returns an SSE event stream.
// POST /api/v1/apps/{app_name}/deploy
func (c *Client) CreateDeploy(ctx context.Context, appName string, input RemoteDeploymentRequest) (<-chan *DeploymentEvent, error) {
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/apps/%s/deploy", c.baseUrl, appName)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(input); err != nil {
		return nil, fmt.Errorf("failed to encode request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")
	// req.Header.Add("Accept", "text/event-stream")

	out := make(chan *DeploymentEvent)

	go func() {
		defer close(out)

		res, err := c.httpClient.Do(req)
		if err != nil {
			return
		}
		defer res.Body.Close()

		if res.StatusCode < 200 || res.StatusCode >= 300 {
			_, _ = io.ReadAll(res.Body)
			return
		}

		for ev, err := range sse.Read(res.Body, nil) {
			if err != nil {
				if err == io.EOF {
					return
				}

				// TODO(AG): Handle error
				fmt.Println("error", err)
				return
			}

			evt, err := UnmarshalDeploymentEvent([]byte(ev.Data))
			if err != nil {
				// TODO(AG): Handle error
				fmt.Println("error", err)
				return
			}
			out <- evt
		}

	}()

	return out, nil
}
