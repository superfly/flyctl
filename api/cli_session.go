package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type CLISession struct {
	ID          string                 `json:"id"`
	URL         string                 `json:"auth_url,omitempty"`
	AccessToken string                 `json:"access_token,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// StartCLISession starts a session with the platform via web
func StartCLISession(sessionName string, args map[string]interface{}) (CLISession, error) {
	var result CLISession

	if args == nil {
		args = make(map[string]interface{})
	}
	args["name"] = sessionName

	postData, _ := json.Marshal(args)

	url := fmt.Sprintf("%s/api/v1/cli_sessions", baseURL)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(postData))
	if err != nil {
		return result, err
	}

	if resp.StatusCode != 201 {
		return result, ErrUnknown
	}

	defer resp.Body.Close() //skipcq: GO-S2307

	json.NewDecoder(resp.Body).Decode(&result)

	return result, nil
}

func GetCLISessionState(ctx context.Context, id string) (CLISession, error) {

	var value CLISession

	url := fmt.Sprintf("%s/api/v1/cli_sessions/%s", baseURL, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return value, err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return value, err
	}
	defer res.Body.Close() //skipcq: GO-S2307

	switch res.StatusCode {
	case http.StatusOK:
		var auth CLISession
		if err = json.NewDecoder(res.Body).Decode(&auth); err != nil {
			return value, fmt.Errorf("failed to decode session, please try again: %w", err)
		}
		return auth, nil
	case http.StatusNotFound:
		return value, ErrNotFound
	default:
		return value, ErrUnknown
	}
}
