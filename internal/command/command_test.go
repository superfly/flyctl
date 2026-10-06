package command

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagnames"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/iostreams"
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
