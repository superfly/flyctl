package api

import (
	"context"
	"fmt"
	"strings"
)

type CLISessionAuth struct {
	CLISession
}

// StartCLISessionWebAuth starts a session with the platform via web auth
func StartCLISessionWebAuth(machineName string, signup bool) (CLISession, error) {

	return StartCLISession(machineName, map[string]interface{}{
		"signup": signup,
		"target": "auth",
	})
}

// GetAccessTokenForCLISession Obtains the access token for the session
func GetAccessTokenForCLISession(ctx context.Context, id string) (string, error) {
	val, err := GetCLISessionState(ctx, id)
	if err != nil {
		return "", err
	}
	return val.AccessToken, nil
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
