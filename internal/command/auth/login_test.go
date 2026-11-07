package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/tokens"

	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

// loginTestContext builds the context runLogin expects, with --code set.
func loginTestContext(t *testing.T, dir, apiBaseURL, code string) (context.Context, *bytes.Buffer) {
	t.Helper()

	fs := pflag.NewFlagSet("login", pflag.ContinueOnError)
	fs.Bool("interactive", false, "")
	for _, name := range []string{"email", "password", "otp", "code"} {
		fs.String(name, "", "")
	}
	require.NoError(t, fs.Set("code", code))

	streams, _, stdout, stderr := iostreams.Test()
	ctx := flag.NewContext(t.Context(), fs)
	ctx = config.NewContext(ctx, &config.Config{
		APIBaseURL: apiBaseURL,
		Tokens:     new(tokens.Tokens),
	})
	ctx = state.WithConfigDirectory(ctx, dir)
	ctx = state.WithHostname(ctx, "test-host")
	ctx = iostreams.NewContext(ctx, streams)
	ctx = logger.NewContext(ctx, logger.New(stderr, logger.Info, false))

	return ctx, stdout
}

func TestRunLoginWithCode(t *testing.T) {
	dir := logoutTestDir(t)
	t.Setenv("FLY_AGENT_SOCKET_PATH", "") // SaveToken stops the agent; keep it to the scratch dir

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/cli_sessions/sess1/redeem":
			var body struct {
				Code         string `json:"code"`
				CodeVerifier string `json:"code_verifier"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Code != "the-code" || body.CodeVerifier != "the-verifier" {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"error":"invalid_code"}`)
				return
			}
			fmt.Fprint(w, `{"id":"sess1","access_token":"tok123","pkce":true}`)
		case "/graphql":
			fmt.Fprint(w, `{"data":{"viewer":{"__typename":"User","id":"u1","email":"agent@example.com"}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	fly.SetBaseURL(ts.URL)

	// An earlier `fly auth login` left this session waiting for its code.
	pendingPath := filepath.Join(dir, "pending-login.json")
	require.NoError(t, os.WriteFile(pendingPath, []byte(`{"id":"sess1","verifier":"the-verifier","expires_at":"2999-01-01T00:00:00Z"}`), 0o600))

	ctx, stdout := loginTestContext(t, dir, ts.URL, "the-code")

	require.NoError(t, runLogin(ctx))

	token, err := config.ReadAccessToken(filepath.Join(dir, config.FileName))
	require.NoError(t, err)
	assert.Equal(t, "tok123", token)
	assert.Contains(t, stdout.String(), "agent@example.com")
	assert.NoFileExists(t, pendingPath)
}
