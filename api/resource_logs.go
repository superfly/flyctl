package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

func (c *Client) GetAppLogs(appName string, nextToken string, region string, instanceId string) ([]LogEntry, string, error) {

	data := url.Values{}
	data.Set("next_token", nextToken)
	if instanceId != "" {
		data.Set("instance", instanceId)
	}
	if region != "" {
		data.Set("region", region)
	}

	url := fmt.Sprintf("%s/api/v1/apps/%s/logs?%s", baseURL, appName, data.Encode())

	req, err := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))

	var result getLogsResponse

	entries := []LogEntry{}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return entries, "", err
	}

	if resp.StatusCode != 200 {
		return entries, "", ErrorFromResp(resp)
	}

	defer resp.Body.Close()

	json.NewDecoder(resp.Body).Decode(&result)

	for _, d := range result.Data {
		entries = append(entries, d.Attributes)
	}

	return entries, result.Meta.NextToken, nil
}
