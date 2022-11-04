package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/superfly/flyctl/internal/logger"
)

type getLogsResponse struct {
	Data []struct {
		Id         string
		Attributes LogEntry
	}
	Meta struct {
		NextToken string `json:"next_token"`
	}
}

func (c *Client) GetAppLogs(ctx context.Context, appName, token, region, instanceID string) (entries []LogEntry, nextToken string, err error) {
	logger := logger.MaybeFromContext(ctx)

	httpClient, err := NewHTTPClient(logger, http.DefaultTransport)

	if err != nil {
		return
	}

	data := url.Values{}
	data.Set("next_token", token)
	if instanceID != "" {
		data.Set("instance", instanceID)
	}
	if region != "" {
		data.Set("region", region)
	}

	url := fmt.Sprintf("%s/api/v1/apps/%s/logs?%s", baseURL, appName, data.Encode())

	var req *http.Request
	if req, err = http.NewRequestWithContext(ctx, "GET", url, nil); err != nil {
		return
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))
	if c.trace != "" {
		req.Header.Set("Fly-Force-Trace", c.trace)
	}

	var result getLogsResponse

	var res *http.Response
	if res, err = httpClient.Do(req); err != nil {
		return
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		err = ErrorFromResp(res)

		return
	}

	if err = json.NewDecoder(res.Body).Decode(&result); err == nil {
		nextToken = result.Meta.NextToken

		for _, d := range result.Data {
			entries = append(entries, d.Attributes)
		}
	}

	return
}
