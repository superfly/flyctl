package spritestate

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/superfly/fly-go/tokens"

	"github.com/superfly/flyctl/internal/config"
)

// tokenDescription labels the tokens flyctl mints, so they can be told apart
// from others in the organization's token list.
const tokenDescription = "flyctl"

// tokenClient talks to the token endpoints, which are authenticated with the
// Fly.io login rather than a Sprites token. It speaks HTTP/1.1 only: a login
// with many organizations makes an Authorization header larger than the edge
// accepts over HTTP/2.
func tokenClient() *http.Client {
	transport := cleanhttp.DefaultTransport()
	transport.ForceAttemptHTTP2 = false
	transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}

	return &http.Client{Transport: transport, Timeout: 30 * time.Second}
}

func tokenRequest(ctx context.Context, method, apiURL, org, flyMacaroon string, body any) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/v1/organizations/%s/tokens", strings.TrimRight(apiURL, "/"), url.PathEscape(org))

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "FlyV1 "+flyMacaroon)
	req.Header.Set("Content-Type", "application/json")

	return tokenClient().Do(req)
}

func tokenError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	var parsed struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}

	if json.Unmarshal(body, &parsed) == nil {
		switch {
		case parsed.Message != "":
			return fmt.Errorf("%s (status %d)", parsed.Message, resp.StatusCode)
		case parsed.Error != "":
			return fmt.Errorf("%s (status %d)", parsed.Error, resp.StatusCode)
		}
	}

	return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

// MintToken exchanges a Fly.io macaroon for a Sprites API token in org.
func MintToken(ctx context.Context, apiURL, org, flyMacaroon string) (string, error) {
	resp, err := tokenRequest(ctx, http.MethodPost, apiURL, org, flyMacaroon,
		map[string]string{"description": tokenDescription})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", tokenError(resp)
	}

	var result struct {
		Token string `json:"token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to read the token response: %w", err)
	}

	if result.Token == "" {
		return "", errors.New("the API returned no token")
	}

	return result.Token, nil
}

// RevokeToken deletes a Sprites API token minted in org. A token that is
// already gone counts as revoked.
func RevokeToken(ctx context.Context, apiURL, org, flyMacaroon, token string) error {
	resp, err := tokenRequest(ctx, http.MethodDelete, apiURL, org, flyMacaroon,
		map[string]string{"token": token})
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusNotFound:
		return nil
	default:
		return tokenError(resp)
	}
}

// RevokeAll revokes every saved token and then removes the state file. It
// returns the tokens' organizations that could not be revoked; the file is
// removed regardless, since the login that could revoke them is going away.
func RevokeAll(ctx context.Context, configDir, flyMacaroon string) (failed []string, err error) {
	st, err := Load(configDir)
	if err != nil {
		return nil, err
	}

	for key, token := range st.Tokens {
		apiURL, org, ok := strings.Cut(key, "|")
		if !ok || token == "" {
			continue
		}

		if flyMacaroon == "" || RevokeToken(ctx, apiURL, org, flyMacaroon, token) != nil {
			failed = append(failed, org)
		}
	}

	return failed, Clear(configDir)
}

// RevokeForLoginChange revokes and forgets the saved tokens before the Fly.io
// login saved in configFile is replaced or removed, since that login minted
// them and is the only one that can revoke them. It never fails the login
// change: what went wrong comes back as warnings for the caller to print.
func RevokeForLoginChange(ctx context.Context, configDir, configFile string) (warnings []string) {
	var macaroon string
	if saved, err := config.ReadAccessToken(configFile); err == nil {
		macaroon = tokens.Parse(saved).Flaps()
	} else if !errors.Is(err, fs.ErrNotExist) {
		warnings = append(warnings, fmt.Sprintf("Failed reading the login for Sprites token revocation: %v", err))
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	failed, err := RevokeAll(ctx, configDir, macaroon)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("Failed removing saved Sprites tokens: %v", err))
	}

	if len(failed) > 0 {
		sort.Strings(failed)
		warnings = append(warnings, fmt.Sprintf(
			"Sprites API tokens for %s could not be revoked, so they remain valid. Revoke them in the Sprites dashboard.",
			strings.Join(failed, ", "),
		))
	}

	return warnings
}
