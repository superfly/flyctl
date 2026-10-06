package webauth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
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
// runs flyctl: stdin is not a TTY, no CI markers are set, and no browser opens.
func headlessContext(t *testing.T) (context.Context, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	for _, key := range []string{"CI", "GITHUB_ACTIONS", "CONTINUOUS_INTEGRATION", "BUILD_NUMBER", "RUN_ID"} {
		t.Setenv(key, "") // restores the original value after the test
		os.Unsetenv(key)
	}
	t.Setenv("PATH", t.TempDir()) // no open/xdg-open to start a browser

	ios, _, out, errOut := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), ios)
	ctx = logger.NewContext(ctx, logger.New(ios.ErrOut, logger.Info, false))
	ctx = state.WithHostname(ctx, "test-host")
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	t.Cleanup(cancel)

	return ctx, out, errOut
}

func TestRunWebLoginHeadless(t *testing.T) {
	ctx, out, errOut := headlessContext(t)

	var (
		mu      sync.Mutex
		args    map[string]any
		created = make(chan struct{})
	)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/cli_sessions":
			if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id":"sess1","auth_url":"https://fly.example/app/auth/cli/sess1","pkce":true}`)
			close(created)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/cli_sessions/sess1/redeem":
			var body struct {
				Code         string `json:"code"`
				CodeVerifier string `json:"code_verifier"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			sum := sha256.Sum256([]byte(body.CodeVerifier))
			if body.Code != "the-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != args["code_challenge"] {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"error":"invalid_code"}`)
				return
			}
			fmt.Fprint(w, `{"id":"sess1","access_token":"tok123","pkce":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	fly.SetBaseURL(ts.URL)

	type result struct {
		token string
		err   error
	}
	done := make(chan result, 1)
	go func() {
		token, err := RunWebLogin(ctx, false)
		done <- result{token, err}
	}()

	select {
	case <-created:
	case r := <-done:
		t.Fatalf("login ended before starting a session: %v", r.err)
	case <-ctx.Done():
		t.Fatal("no login session was started")
	}

	// A browser on this machine approves and calls back on the loopback.
	mu.Lock()
	callback := fmt.Sprintf("http://127.0.0.1:%v/callback?code=the-code&state=%s", args["redirect_port"], url.QueryEscape(fmt.Sprint(args["state"])))
	mu.Unlock()
	res, err := http.Get(callback)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	var r result
	select {
	case r = <-done:
	case <-ctx.Done():
		t.Fatal("login did not finish after the callback")
	}
	if r.err != nil || r.token != "tok123" {
		t.Fatalf("expected token tok123, got %q, %v", r.token, r.err)
	}
	if strings.Contains(out.String(), "paste code here") {
		t.Fatalf("prompted for a paste without a terminal: %q", out.String())
	}
	if !strings.Contains(out.String()+errOut.String(), "https://fly.example/app/auth/cli/sess1") {
		t.Fatalf("login URL not printed; stdout %q, stderr %q", out.String(), errOut.String())
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
