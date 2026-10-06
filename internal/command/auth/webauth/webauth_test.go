package webauth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

// headlessContext sets up a run without a terminal, the way a coding agent
// runs flyctl: stdin is not a TTY, no CI markers are set, no browser opens,
// and the config directory is a scratch one.
func headlessContext(t *testing.T) (context.Context, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	for _, key := range []string{"CI", "GITHUB_ACTIONS", "CONTINUOUS_INTEGRATION", "BUILD_NUMBER", "RUN_ID"} {
		t.Setenv(key, "") // restores the original value after the test
		os.Unsetenv(key)
	}
	t.Setenv("PATH", t.TempDir()) // no open/xdg-open to start a browser

	dir := t.TempDir()
	t.Chdir(dir) // the config lock file lands in the working directory under tests

	ios, _, out, errOut := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), ios)
	ctx = logger.NewContext(ctx, logger.New(ios.ErrOut, logger.Info, false))
	ctx = state.WithHostname(ctx, "test-host")
	ctx = state.WithConfigDirectory(ctx, dir)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	t.Cleanup(cancel)

	return ctx, out, errOut
}

// loginServer fakes the CLI session endpoints: it records the create request
// and redeems "the-code" for "tok123" when the verifier matches the challenge.
type loginServer struct {
	*httptest.Server
	created chan struct{}

	mu   sync.Mutex
	args map[string]any
}

func newLoginServer(t *testing.T) *loginServer {
	t.Helper()

	s := &loginServer{created: make(chan struct{})}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/cli_sessions":
			if err := json.NewDecoder(r.Body).Decode(&s.args); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id":"sess1","auth_url":"https://fly.example/app/auth/cli/sess1","pkce":true}`)
			close(s.created)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/cli_sessions/sess1/redeem":
			var body struct {
				Code         string `json:"code"`
				CodeVerifier string `json:"code_verifier"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			sum := sha256.Sum256([]byte(body.CodeVerifier))
			if body.Code != "the-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != s.args["code_challenge"] {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"error":"invalid_code"}`)
				return
			}
			fmt.Fprint(w, `{"id":"sess1","access_token":"tok123","pkce":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	fly.SetBaseURL(s.URL)

	return s
}

func (s *loginServer) waitCreated(t *testing.T, ctx context.Context) {
	t.Helper()

	select {
	case <-s.created:
	case <-ctx.Done():
		t.Fatal("no login session was started")
	}
}

// approve plays the browser on this machine: it calls back on the CLI's
// loopback port with the completion code.
func (s *loginServer) approve(t *testing.T, ctx context.Context) {
	t.Helper()

	s.waitCreated(t, ctx)

	s.mu.Lock()
	callback := fmt.Sprintf("http://127.0.0.1:%v/callback?code=the-code&state=%s", s.args["redirect_port"], url.QueryEscape(fmt.Sprint(s.args["state"])))
	s.mu.Unlock()

	res, err := http.Get(callback)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
}

// waitForPendingLogin polls until RunWebLogin has saved its pending login.
func waitForPendingLogin(t *testing.T, path string) pendingLogin {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		p, err := loadPendingLogin(path)
		if err == nil {
			return p
		}
		if time.Now().After(deadline) {
			t.Fatalf("no pending login saved: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func awaitLogin(t *testing.T, ctx context.Context, done <-chan waitResult) waitResult {
	t.Helper()

	select {
	case r := <-done:
		return r
	case <-ctx.Done():
		t.Fatal("login did not finish")

		return waitResult{}
	}
}

func TestRunWebLoginHeadless(t *testing.T) {
	ctx, out, errOut := headlessContext(t)
	pendingPath := filepath.Join(state.ConfigDirectory(ctx), "pending-login.json")
	s := newLoginServer(t)

	done := make(chan waitResult, 1)
	go func() {
		token, err := RunWebLogin(ctx, false)
		done <- waitResult{token, err}
	}()

	// While it waits for approval, the session is saved for `fly auth login --code`.
	s.waitCreated(t, ctx)
	pending := waitForPendingLogin(t, pendingPath)
	sum := sha256.Sum256([]byte(pending.Verifier))
	s.mu.Lock()
	challenge := s.args["code_challenge"]
	s.mu.Unlock()
	if pending.ID != "sess1" || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
		t.Fatalf("saved pending login doesn't match the session: %+v", pending)
	}
	if left := time.Until(pending.ExpiresAt); left <= 0 || left > loginTimeout {
		t.Fatalf("pending login expires in %v, want within %v", left, loginTimeout)
	}

	s.approve(t, ctx)

	r := awaitLogin(t, ctx, done)
	if r.err != nil || r.token != "tok123" {
		t.Fatalf("expected token tok123, got %q, %v", r.token, r.err)
	}
	if strings.Contains(out.String(), "paste code here") {
		t.Fatalf("prompted for a paste without a terminal: %q", out.String())
	}
	if !strings.Contains(out.String()+errOut.String(), "https://fly.example/app/auth/cli/sess1") {
		t.Fatalf("login URL not printed; stdout %q, stderr %q", out.String(), errOut.String())
	}
	if _, err := os.Stat(pendingPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending login left behind after a successful login: %v", err)
	}
}

func TestRunWebLoginCannotSavePendingLogin(t *testing.T) {
	ctx, _, errOut := headlessContext(t)
	ctx = state.WithConfigDirectory(ctx, filepath.Join(t.TempDir(), "missing"))
	s := newLoginServer(t)

	old := pendingCheckInterval
	pendingCheckInterval = 10 * time.Millisecond
	t.Cleanup(func() { pendingCheckInterval = old })

	done := make(chan waitResult, 1)
	go func() {
		token, err := RunWebLogin(ctx, false)
		done <- waitResult{token, err}
	}()

	// Leave time for many checks: with nothing saved, a missing file must not
	// read as a login finished or dropped elsewhere.
	s.waitCreated(t, ctx)
	time.Sleep(100 * time.Millisecond)
	s.approve(t, ctx)

	r := awaitLogin(t, ctx, done)
	if r.err != nil || r.token != "tok123" {
		t.Fatalf("expected the login to finish anyway, got %q, %v", r.token, r.err)
	}
	if !strings.Contains(errOut.String(), "won't be able to finish") {
		t.Fatalf("no warning that --code can't finish this login: %q", errOut.String())
	}
}

func TestRunWebLoginHeadlessOnCI(t *testing.T) {
	ctx, _, _ := headlessContext(t)
	t.Setenv("CI", "true")

	var requests atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	defer ts.Close()
	fly.SetBaseURL(ts.URL)

	_, err := RunWebLogin(ctx, false)

	if err == nil || !strings.Contains(err.Error(), "FLY_API_TOKEN") {
		t.Fatalf("expected an error pointing at FLY_API_TOKEN, got %v", err)
	}
	if n := requests.Load(); n != 0 {
		t.Fatalf("started a login on CI, where nobody can approve it (%d requests)", n)
	}
}

func TestRunWebLoginHeadlessLegacyServer(t *testing.T) {
	ctx, _, _ := headlessContext(t)

	var polls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/cli_sessions":
			// A server that predates PKCE leaves "pkce" out.
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id":"sess1","auth_url":"https://fly.example/app/auth/cli/sess1"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/cli_sessions/sess1":
			polls.Add(1)
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	fly.SetBaseURL(ts.URL)

	_, err := RunWebLogin(ctx, false)

	if err == nil || !strings.Contains(err.Error(), "FLY_API_TOKEN") {
		t.Fatalf("expected an error pointing at FLY_API_TOKEN, got %v", err)
	}
	if n := polls.Load(); n != 0 {
		t.Fatalf("polled a pre-PKCE session for its token without a terminal (%d polls)", n)
	}
}
