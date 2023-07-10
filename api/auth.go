package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
func GetAccessTokenForCLISession(ctx context.Context, id string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/cli_sessions/%s", baseURL, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		var auth CLISessionAuth
		if err = json.NewDecoder(res.Body).Decode(&auth); err != nil {
			return "", fmt.Errorf("Failed to decode auth token, please try again: %w", err)
		}
		return auth.AccessToken, nil
	case http.StatusNotFound:
		return "", ErrNotFound
	default:
		return "", ErrUnknown
	}
}

const flyv1Scheme = "FlyV1"

func AuthorizationHeader(token string) string {
	token = strings.TrimSpace(token)

	if scheme, _, ok := strings.Cut(token, " "); ok && scheme == flyv1Scheme {
		return token
	}

	// macaroon without scheme
	if strings.HasPrefix(token, "fm1r_") {
		return strings.Join([]string{flyv1Scheme, token}, " ")
	}

	return fmt.Sprintf("Bearer %s", token)
}
