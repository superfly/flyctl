package api

import (
	"bytes"
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
func GetAccessTokenForCLISession(id string) (CLISessionAuth, error) {
	var result CLISessionAuth

	url := fmt.Sprintf("%s/api/v1/cli_sessions/%s", baseURL, id)

	resp, err := http.Get(url)
	if err != nil {
		return result, err
	}

	if resp.StatusCode == 404 {
		return result, ErrNotFound
	}

	if resp.StatusCode != 200 {
		return result, ErrUnknown
	}

	defer resp.Body.Close()

	json.NewDecoder(resp.Body).Decode(&result)

	return result, nil
}
