package api

import (
	"context"
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

func AuthorizationHeader(token string) string {
	for _, tok := range strings.Split(token, ",") {
		switch pfx, _, _ := strings.Cut(tok, "_"); pfx {
		case "fm1r", "fm2":
			return "FlyV1 " + token
		}
	}

	return "Bearer " + token
}
