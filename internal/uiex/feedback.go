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

// FeedbackRequest is the body of POST /api/v1/feedback. At least one of
// RequestID or URL must be set.
type FeedbackRequest struct {
	Category  string `json:"category"`
	Note      string `json:"note"`
	RequestID string `json:"request_id,omitempty"`
	URL       string `json:"url,omitempty"`
}

type FeedbackResponse struct {
	FeedbackID       string `json:"feedback_id"`
	AlreadySubmitted bool   `json:"already_submitted"`
}

// SubmitFeedback files a feedback report. The API deduplicates identical
// reports, returning the original ID with AlreadySubmitted set.
func (c *Client) SubmitFeedback(ctx context.Context, in FeedbackRequest) (*FeedbackResponse, error) {
	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/feedback", c.baseUrl)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(in); err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
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
		var out FeedbackResponse
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("failed to decode response, please try again: %w", err)
		}

		return &out, nil
	case http.StatusBadRequest:
		return nil, fmt.Errorf("the feedback API doesn't accept legacy tokens; run `fly auth login` to get a new one (status %d): %s", res.StatusCode, string(body))
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("not authorized to submit feedback; run `fly auth login` and try again (status %d): %s", res.StatusCode, string(body))
	case http.StatusTooManyRequests:
		return nil, fmt.Errorf("too many feedback reports from your organization; wait a minute and try again (status %d): %s", res.StatusCode, string(body))
	default:
		return nil, fmt.Errorf("failed to submit feedback (status %d): %s", res.StatusCode, string(body))
	}
}
