package tokens

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagnames"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/iostreams"
)

type revokeTransport func(*http.Request) (*http.Response, error)

func (f revokeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRevokeSupplied(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		noToken    bool
		bothEnv    bool
		savedOnly  bool
		blockedAPI bool
		wantError  string
	}{
		{name: "success", body: `{"data":{"logOut":{"ok":true}}}`},
		{name: "both environment variables", bothEnv: true, body: `{"data":{"logOut":{"ok":true}}}`},
		{name: "rejected", body: `{"data":{"logOut":{"ok":false}}}`, wantError: "did not accept"},
		{name: "API error", body: `{"errors":[{"message":"permission denied"}]}`, wantError: "permission denied"},
		{name: "transport error", wantError: "connection failed"},
		{name: "no token", noToken: true, wantError: "no access token selected"},
		{name: "saved token is not revoked", savedOnly: true, wantError: "Use `fly auth logout`"},
		{name: "empty access env blocks API and protects saved token", savedOnly: true, blockedAPI: true, wantError: "No access token supplied by --access-token, FLY_ACCESS_TOKEN, or FLY_API_TOKEN."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(config.AccessTokenEnvKey, "")
			t.Setenv(config.APITokenEnvKey, "")
			if tc.bothEnv {
				t.Setenv(config.AccessTokenEnvKey, "test-token")
				t.Setenv(config.APITokenEnvKey, "other-token")
			}
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			fs.String(flagnames.AccessToken, "", "")
			if !tc.bothEnv && !tc.savedOnly {
				value := "test-token"
				if tc.noToken {
					value = ""
				}
				require.NoError(t, fs.Set(flagnames.AccessToken, value))
			}
			if tc.blockedAPI {
				t.Setenv(config.APITokenEnvKey, "other-token")
			}
			ctx := flag.NewContext(context.Background(), fs)
			streams, _, stdout, stderr := iostreams.Test()
			ctx = iostreams.NewContext(ctx, streams)
			token := "test-token"
			if tc.noToken {
				token = ""
			}
			calls := 0
			client := fly.NewClientFromOptions(fly.ClientOptions{
				AccessToken: token, BaseURL: "https://example.invalid",
				Transport: &fly.Transport{UnderlyingTransport: revokeTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
					require.Equal(t, "/graphql", r.URL.Path)
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					require.Contains(t, string(body), "logOut(input: {})")
					if tc.body == "" {
						return nil, errors.New("connection failed")
					}
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
				})},
			})
			ctx = flyutil.NewContextWithClient(ctx, client)
			err := runRevokeSupplied(ctx)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.Empty(t, stdout.String())
			} else {
				require.NoError(t, err)
				if tc.bothEnv {
					require.Contains(t, stdout.String(), "from FLY_ACCESS_TOKEN")
					require.Contains(t, stderr.String(), "FLY_API_TOKEN was not targeted")
					require.Contains(t, stderr.String(), "Unset both variables")
					require.Equal(t, "test-token", os.Getenv(config.AccessTokenEnvKey))
					require.Equal(t, "other-token", os.Getenv(config.APITokenEnvKey))
				} else {
					require.Contains(t, stdout.String(), "from --access-token")
					require.Contains(t, stderr.String(), "Stop passing this token")
				}
			}
			if tc.noToken || tc.savedOnly {
				require.Zero(t, calls)
			} else {
				require.Equal(t, 1, calls)
			}
			require.NotContains(t, stdout.String()+stderr.String(), "test-token")
			require.NotContains(t, stdout.String()+stderr.String(), "other-token")
		})
	}
}

func TestSuppliedTokenSource(t *testing.T) {
	for _, tc := range []struct {
		name        string
		access      string
		api         string
		want        string
		unsetAccess bool
		flagSet     bool
	}{
		{name: "config", want: ""},
		{name: "API env", unsetAccess: true, api: "api", want: config.APITokenEnvKey},
		{name: "both env", access: "access", api: "api", want: config.AccessTokenEnvKey},
		{name: "empty access blocks API", api: "api", want: ""},
		{name: "flag overrides both", flagSet: true, access: "access", api: "api", want: "--access-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(config.AccessTokenEnvKey, tc.access)
			t.Setenv(config.APITokenEnvKey, tc.api)
			if tc.unsetAccess {
				require.NoError(t, os.Unsetenv(config.AccessTokenEnvKey))
			}
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			fs.String(flagnames.AccessToken, "", "")
			if tc.flagSet {
				require.NoError(t, fs.Set(flagnames.AccessToken, "flag-token"))
			}
			require.Equal(t, tc.want, suppliedTokenSource(flag.NewContext(t.Context(), fs)))
		})
	}
}
