package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// CLISessionAuth holds access information
type CLISessionAuth struct {
	ID          string `json:"id"`
	AuthURL     string `json:"auth_url"`
	AccessToken string `json:"access_token"`
}

// StartCLISessionWebAuth starts a session with the platform via web auth
func StartCLISessionWebAuth(machineName string, signup bool) (CLISessionAuth, error) {
	var result CLISessionAuth

	postData, _ := json.Marshal(map[string]interface{}{
		"name":   machineName,
		"signup": signup,
	})

	url := fmt.Sprintf("%s/api/v1/cli_sessions", baseURL)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(postData))
	if err != nil {
		return result, err
	}

	if resp.StatusCode != 201 {
		return result, ErrUnknown
	}

	defer resp.Body.Close()

	json.NewDecoder(resp.Body).Decode(&result)

	return result, nil
}

// GetAccessTokenForCLISession Obtains the access token for the session
func GetAccessTokenForCLISession(ctx context.Context, id string) (token string, err error) {
	url := fmt.Sprintf("%s/api/v1/cli_sessions/%s", baseURL, id)

	var req *http.Request
	if req, err = http.NewRequestWithContext(ctx, http.MethodGet, url, nil); err != nil {
		return
	}

	var res *http.Response
	if res, err = http.DefaultClient.Do(req); err != nil {
		return
	}
	defer res.Body.Close()

	switch res.StatusCode {
	default:
		err = ErrUnknown
	case http.StatusNotFound:
		err = ErrNotFound
	case http.StatusOK:
		var auth CLISessionAuth

		if err = json.NewDecoder(res.Body).Decode(&auth); err == nil {
			token = auth.AccessToken
		}
	}

	return
}
