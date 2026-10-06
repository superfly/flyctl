package webauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/state"
)

func TestPendingLoginFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending-login.json")
	want := pendingLogin{ID: "sess1", Verifier: "verifier1", ExpiresAt: time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)}

	if err := savePendingLogin(path, want); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("pending login is readable by others: mode %o", mode)
	}

	got, err := loadPendingLogin(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.Verifier != want.Verifier || !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Fatalf("loaded %+v, saved %+v", got, want)
	}

	// A new login replaces the old one.
	if err := savePendingLogin(path, pendingLogin{ID: "sess2", Verifier: "verifier2", ExpiresAt: want.ExpiresAt}); err != nil {
		t.Fatal(err)
	}
	if got, _ := loadPendingLogin(path); got.ID != "sess2" {
		t.Fatalf("expected the newer login sess2, got %q", got.ID)
	}
}

func TestLoadPendingLoginErrors(t *testing.T) {
	dir := t.TempDir()

	if _, err := loadPendingLogin(filepath.Join(dir, "missing.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist for a missing file, got %v", err)
	}

	for name, content := range map[string]string{
		"corrupt":    "{not json",
		"incomplete": `{"id":"sess1"}`,
	} {
		path := filepath.Join(dir, name+".json")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadPendingLogin(path); err == nil || errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s file: expected a read error, got %v", name, err)
		}
	}
}

func TestRemovePendingLogin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending-login.json")
	p := pendingLogin{ID: "sess1", Verifier: "verifier1", ExpiresAt: time.Now().Add(time.Minute)}

	// A file holding another session stays: a newer login replaced ours.
	if err := savePendingLogin(path, p); err != nil {
		t.Fatal(err)
	}
	removePendingLogin(path, "sess-other")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("removed a pending login that belongs to another session: %v", err)
	}

	removePendingLogin(path, "sess1")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected our pending login removed, stat: %v", err)
	}

	// Nothing can finish an unreadable file, so it goes too.
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	removePendingLogin(path, "sess1")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected the unreadable file removed, stat: %v", err)
	}
}

// redeemTestContext returns a context with a scratch config directory and the
// path of the pending login file in it.
func redeemTestContext(t *testing.T) (context.Context, string) {
	t.Helper()

	dir := t.TempDir()

	return state.WithConfigDirectory(context.Background(), dir), filepath.Join(dir, "pending-login.json")
}

func TestRedeemPendingLogin(t *testing.T) {
	cases := []struct {
		name        string
		status      int    // redeem response status; 0 means no request may be made
		body        string // redeem response body
		expiresIn   time.Duration
		wantToken   string
		wantErr     string
		wantRemoved bool // pending file gone after the call
	}{
		{name: "approved", status: 200, body: `{"id":"sess1","access_token":"tok123","pkce":true}`, expiresIn: time.Minute, wantToken: "tok123"},
		{name: "wrong code", status: 403, body: `{"error":"invalid_code"}`, expiresIn: time.Minute, wantErr: "didn't work"},
		{name: "not approved yet", status: 400, body: `{"error":"authorization_pending"}`, expiresIn: time.Minute, wantErr: "approve"},
		{name: "expired on the server", status: 404, body: `{"error":"not found"}`, expiresIn: time.Minute, wantErr: "expired", wantRemoved: true},
		{name: "expired locally", expiresIn: -time.Minute, wantErr: "expired", wantRemoved: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, path := redeemTestContext(t)
			if err := savePendingLogin(path, pendingLogin{ID: "sess1", Verifier: "verifier1", ExpiresAt: time.Now().Add(tc.expiresIn)}); err != nil {
				t.Fatal(err)
			}

			var requests atomic.Int32
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				var body struct {
					Code         string `json:"code"`
					CodeVerifier string `json:"code_verifier"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if r.URL.Path != "/api/v1/cli_sessions/sess1/redeem" || body.Code != "the-code" || body.CodeVerifier != "verifier1" {
					t.Errorf("unexpected redeem request: %s %+v", r.URL.Path, body)
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer ts.Close()
			fly.SetBaseURL(ts.URL)

			// Whitespace from copying the code off the page is ignored.
			token, finish, err := RedeemPendingLogin(ctx, " the-code\n")

			_, statErr := os.Stat(path)
			removed := errors.Is(statErr, os.ErrNotExist)

			if tc.wantErr == "" {
				if err != nil || token != tc.wantToken {
					t.Fatalf("expected token %q, got %q, %v", tc.wantToken, token, err)
				}
				if removed {
					t.Fatal("pending login removed before the token was saved")
				}
				finish()
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("finish left the pending login behind: %v", err)
				}

				return
			}

			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected an error containing %q, got %v", tc.wantErr, err)
			}
			if tc.status == 0 && requests.Load() != 0 {
				t.Fatal("redeemed a login that had already expired")
			}
			if removed != tc.wantRemoved {
				t.Fatalf("pending login removed = %v, want %v", removed, tc.wantRemoved)
			}
		})
	}
}

func TestRedeemPendingLoginWithoutPendingLogin(t *testing.T) {
	ctx, _ := redeemTestContext(t)

	_, _, err := RedeemPendingLogin(ctx, "the-code")

	if err == nil || !strings.Contains(err.Error(), "fly auth login") {
		t.Fatalf("expected an error pointing at fly auth login, got %v", err)
	}
}

func TestRedeemPendingLoginUnreadable(t *testing.T) {
	ctx, path := redeemTestContext(t)
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := RedeemPendingLogin(ctx, "the-code")

	if err == nil || !strings.Contains(err.Error(), "fly auth login") {
		t.Fatalf("expected an error pointing at fly auth login, got %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected the unreadable pending login removed: %v", err)
	}
}
