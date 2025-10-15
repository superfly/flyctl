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
	BuilderID    string                   `json:"builder_id"`
}

type RemoteDeploymentResponse struct {
	Events <-chan *DeploymentEvent
	Errors <-chan error
}

// CreateDeploy creates a new remote deploy for the given app and returns an SSE event stream.
// POST /api/v1/apps/{app_name}/deploy
func (c *Client) CreateDeploy(ctx context.Context, appName string, input RemoteDeploymentRequest) (RemoteDeploymentResponse, error) {
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/apps/%s/deploy", c.baseUrl, appName)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(input); err != nil {
		return RemoteDeploymentResponse{}, fmt.Errorf("failed to encode request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return RemoteDeploymentResponse{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")
	// req.Header.Add("Accept", "text/event-stream")

	out := make(chan *DeploymentEvent)
	errors := make(chan error)

	go func() {
		defer close(out)
		defer close(errors)

		res, err := c.httpClient.Do(req)
		if err != nil {
			return
		}
		defer res.Body.Close()

		if res.StatusCode < 200 || res.StatusCode >= 300 {
			body, _ := io.ReadAll(res.Body)
			errors <- fmt.Errorf("unexpected status code received from the deployment server %d: %s", res.StatusCode, string(body))
			return
		}

		for ev, err := range sse.Read(res.Body, nil) {
			if err != nil {
				if err == io.EOF {
					return
				}

				errors <- err
				return
			}

			if ev.Type == "ping" {
				continue
			}

			evt, err := UnmarshalDeploymentEvent([]byte(ev.Data))
			if err != nil {
				errors <- err
				return
			}
			out <- evt
		}

	}()

	return RemoteDeploymentResponse{Events: out, Errors: errors}, nil
}
