package command

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/cmdutil/preparers"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagnames"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/task"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
)

func TestFilesFromCommandUsesPOSIXGuestPath(t *testing.T) {
	content := []byte("hello from windows")
	localPath := filepath.Join(t.TempDir(), "config.txt")
	if err := os.WriteFile(localPath, content, 0o600); err != nil {
		t.Fatal(err)
	}

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.StringArray("file-local", []string{"/etc/config.txt=" + localPath}, "")
	ctx := flag.NewContext(context.Background(), fs)

	files, err := FilesFromCommand(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	if got, want := files[0].GuestPath, "/etc/config.txt"; got != want {
		t.Errorf("GuestPath = %q, want %q", got, want)
	}
	if files[0].RawValue == nil {
		t.Fatal("RawValue is nil")
	}
	if got, want := *files[0].RawValue, base64.StdEncoding.EncodeToString(content); got != want {
		t.Errorf("RawValue = %q, want %q", got, want)
	}
}

func TestHasExternallySuppliedToken(t *testing.T) {
	t.Run("env var set", func(t *testing.T) {
		t.Setenv("FLY_ACCESS_TOKEN", "x")
		ctx := withAccessTokenFlagContext(context.Background(), "")

		if !hasExternallySuppliedToken(ctx) {
			t.Fatal("expected true when FLY_ACCESS_TOKEN is set")
		}
	})

	t.Run("flag set", func(t *testing.T) {
		t.Setenv("FLY_ACCESS_TOKEN", "")
		t.Setenv("FLY_API_TOKEN", "")
		ctx := withAccessTokenFlagContext(context.Background(), "tok")

		if !hasExternallySuppliedToken(ctx) {
			t.Fatal("expected true when --access-token flag is set")
		}
	})

	t.Run("neither set", func(t *testing.T) {
		t.Setenv("FLY_ACCESS_TOKEN", "")
		t.Setenv("FLY_API_TOKEN", "")
		ctx := withAccessTokenFlagContext(context.Background(), "")

		if hasExternallySuppliedToken(ctx) {
			t.Fatal("expected false when neither env nor flag is set")
		}
	})
}

// withAccessTokenFlagContext returns ctx with a flag set that has the
// access-token flag registered (and optionally pre-set). Mirrors how cobra
// constructs the flag context during command preparation.
func withAccessTokenFlagContext(ctx context.Context, value string) context.Context {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.String(flagnames.AccessToken, "", "")
	if value != "" {
		_ = fs.Set(flagnames.AccessToken, value)
	}

	return flag.NewContext(ctx, fs)
}

func TestTryOpenUserURL(t *testing.T) {
	const url = "https://fly.example/sso/start"

	setup := func(t *testing.T, ci bool, tty bool) (context.Context, *bytes.Buffer) {
		for _, key := range []string{"CI", "GITHUB_ACTIONS", "CONTINUOUS_INTEGRATION", "BUILD_NUMBER", "RUN_ID"} {
			t.Setenv(key, "") // restores the original value after the test
			os.Unsetenv(key)
		}
		if ci {
			t.Setenv("CI", "true")
		}
		t.Setenv("PATH", t.TempDir()) // no open/xdg-open to start a browser

		ios, _, _, errOut := iostreams.Test()
		ios.SetStdinTTY(tty)
		ios.SetStdoutTTY(tty)

		return iostreams.NewContext(context.Background(), ios), errOut
	}

	printsURL := func(t *testing.T, errOut *bytes.Buffer) {
		t.Helper()
		for _, line := range strings.Split(errOut.String(), "\n") {
			if strings.TrimSpace(line) == url {
				return
			}
		}
		t.Fatalf("expected the URL on its own line, got %q", errOut.String())
	}

	t.Run("headless", func(t *testing.T) {
		ctx, errOut := setup(t, false, false)

		if err := tryOpenUserURL(ctx, url); err != nil {
			t.Fatalf("expected nil so the sign-in is awaited, got %v", err)
		}
		printsURL(t, errOut)
		if !strings.Contains(errOut.String(), "SSO") {
			t.Fatalf("expected the notice to mention SSO, got %q", errOut.String())
		}
	})

	t.Run("CI", func(t *testing.T) {
		ctx, errOut := setup(t, true, false)

		err := tryOpenUserURL(ctx, url)
		if err == nil || !strings.Contains(err.Error(), "FLY_API_TOKEN") {
			t.Fatalf("expected an error naming FLY_API_TOKEN, got %v", err)
		}
		printsURL(t, errOut)
	})

	t.Run("terminal without a browser", func(t *testing.T) {
		ctx, errOut := setup(t, false, true)

		if err := tryOpenUserURL(ctx, url); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
		printsURL(t, errOut)
	})
}

func TestIncompleteSSO(t *testing.T) {
	const url = "https://fly.example/sso/start"

	ios, _, _, _ := iostreams.Test()
	ctx := logger.NewContext(context.Background(), logger.New(ios.ErrOut, logger.Info, false))
	needs := func(b bool, err error) func() (bool, error) {
		return func() (bool, error) { return b, err }
	}

	if err := incompleteSSO(ctx, "", needs(true, nil)); err != nil {
		t.Fatalf("no URL shown: expected nil, got %v", err)
	}
	if err := incompleteSSO(ctx, url, needs(false, nil)); err != nil {
		t.Fatalf("sign-in finished: expected nil, got %v", err)
	}
	if err := incompleteSSO(ctx, url, needs(false, errors.New("bad token"))); err != nil {
		t.Fatalf("inspection error: expected nil, got %v", err)
	}
	err := incompleteSSO(ctx, url, needs(true, nil))
	if err == nil || !strings.Contains(err.Error(), "SSO sign-in was not completed") || !strings.Contains(err.Error(), url) {
		t.Fatalf("expected an incomplete SSO error naming the URL, got %v", err)
	}
}

func TestRequireSessionWithoutTerminal(t *testing.T) {
	cases := []struct {
		name        string
		token       string
		lastLogin   time.Time
		wantExpired bool // false: no token at all
	}{
		{name: "login older than 30 days", token: "tok", lastLogin: time.Now().Add(-40 * 24 * time.Hour), wantExpired: true},
		{name: "login without a timestamp", token: "tok", wantExpired: true},
		{name: "no login at all", lastLogin: time.Now()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FLY_ACCESS_TOKEN", "")
			t.Setenv("FLY_API_TOKEN", "")

			ios, _, _, _ := iostreams.Test() // no terminal, so nothing prompts
			ctx := iostreams.NewContext(context.Background(), ios)
			ctx = logger.NewContext(ctx, logger.New(ios.ErrOut, logger.Info, false))
			ctx = flag.NewContext(ctx, pflag.NewFlagSet("test", pflag.ContinueOnError))
			ctx = config.NewContext(ctx, &config.Config{LastLogin: tc.lastLogin, Tokens: tokens.Parse(tc.token)})
			ctx = flyutil.NewContextWithClient(ctx, flyutil.NewClientFromOptions(ctx, fly.ClientOptions{Tokens: tokens.Parse(tc.token)}))

			_, err := RequireSession(ctx)

			if !tc.wantExpired {
				if !errors.Is(err, fly.ErrNoAuthToken) {
					t.Fatalf("expected fly.ErrNoAuthToken without a token, got %v", err)
				}
				// The message only names `auth login`; new users and headless
				// runs need signup and FLY_API_TOKEN pointed out, with a
				// tokens command that actually creates one.
				suggestion := flyerr.GetErrorSuggestion(err)
				for _, want := range []string{"fly auth signup", "FLY_API_TOKEN", "fly tokens create deploy"} {
					if !strings.Contains(suggestion, want) {
						t.Fatalf("expected the no-token suggestion to name %q, got %q", want, suggestion)
					}
				}
				return
			}
			// A token is saved, so "no access token available" would be wrong:
			// the session is too old, and logging in again fixes it.
			if err == nil || !strings.Contains(err.Error(), "expired") || !strings.Contains(err.Error(), "fly auth login") {
				t.Fatalf("expected an expired-session error naming fly auth login, got %v", err)
			}
		})
	}
}

func TestSessionAfterDischargeFailure(t *testing.T) {
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"error":"fixture discharge outage"}`)
	}))
	t.Cleanup(endpoint.Close)

	newPermission := func() *macaroon.Macaroon {
		m, err := macaroon.New([]byte("fixture"), flyio.LocationPermission, macaroon.NewSigningKey())
		require.NoError(t, err)
		return m
	}
	encode := func(m *macaroon.Macaroon) string {
		s, err := m.String()
		require.NoError(t, err)
		return s
	}
	key := macaroon.NewEncryptionKey()
	perm := newPermission()
	require.NoError(t, perm.Add3P(key, endpoint.URL))
	ticket, err := perm.ThirdPartyTicket(endpoint.URL)
	require.NoError(t, err)
	_, discharge, err := macaroon.DischargeTicket(key, endpoint.URL, ticket)
	require.NoError(t, err)
	complete := encode(perm) + "," + encode(discharge)
	require.NoError(t, perm.Add3P(key, endpoint.URL+"/second"))
	missing := encode(perm)
	for _, tc := range []struct {
		name        string
		header      string
		prepare     preparers.Preparer
		wantFailure bool
	}{
		{"all missing", missing, RequireFlapsSession, true},
		{"ordinary session during outage", missing, RequireSession, false},
		{"one of two tickets still missing", missing + "," + encode(discharge), RequireFlapsSession, true},
		{"one complete permission", missing + "," + complete, RequireFlapsSession, false},
		{"permission without caveats", missing + "," + encode(newPermission()), RequireFlapsSession, false},
		{"OAuth omitted by Machines client", missing + ",fo1_fixture", RequireFlapsSession, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FLY_ACCESS_TOKEN", tc.header)
			toks := tokens.Parse(tc.header)
			base, cancel := context.WithCancel(t.Context())
			ctx := task.NewWithContext(logger.NewContext(base, logger.New(io.Discard, logger.Info, false)))
			task.FromContext(ctx).Start(ctx)
			defer task.FromContext(ctx).Shutdown()
			defer cancel()
			ctx = config.NewContext(ctx, &config.Config{Tokens: toks})
			ctx = flyutil.NewContextWithClient(ctx, flyutil.NewClientFromOptions(ctx, fly.ClientOptions{Tokens: toks}))
			_, err := tc.prepare(ctx)
			if tc.wantFailure {
				require.ErrorContains(t, err, "failed to refresh third-party discharge tokens")
				require.ErrorContains(t, err, "fixture discharge outage")
				return
			}
			require.NoError(t, err)
		})
	}
}
