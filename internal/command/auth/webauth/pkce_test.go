package webauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/iostreams"
)

// newRedeemServer mocks the server side of the redeem endpoint: it accepts the
// completion code `code` for session `id` when the presented verifier hashes
// to the challenge captured at session create.
func newRedeemServer(t *testing.T, id, code, challenge, token string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/cli_sessions/"+id+"/redeem" {
			http.NotFound(w, r)

			return
		}

		var body struct {
			Code         string `json:"code"`
			CodeVerifier string `json:"code_verifier"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"bad body"}`, http.StatusBadRequest)

			return
		}

		sum := sha256.Sum256([]byte(body.CodeVerifier))
		if body.Code != code || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":"invalid_code"}`)

			return
		}

		fmt.Fprintf(w, `{"id":%q,"access_token":%q,"pkce":true}`, id, token)
	}))
}

func TestPKCELoginCallbackFlow(t *testing.T) {
	args := map[string]any{}

	p, err := newPKCELogin(args)
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()

	challenge, _ := args["code_challenge"].(string)
	if challenge == "" || args["code_challenge_method"] != "S256" || args["state"] == "" {
		t.Fatalf("missing pkce args: %v", args)
	}
	if args["redirect_port"] != p.port || p.port == 0 {
		t.Fatalf("expected bound redirect_port, got %v", args["redirect_port"])
	}

	ts := newRedeemServer(t, "sess1", "the-code", challenge, "tok123")
	defer ts.Close()
	fly.SetBaseURL(ts.URL)

	callback := fmt.Sprintf("http://127.0.0.1:%d/callback", p.port)

	// wrong state is rejected and delivers nothing
	res, err := http.Get(callback + "?code=evil&state=wrong")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad state, got %d", res.StatusCode)
	}

	// the real browser callback
	res, err = http.Get(callback + "?code=the-code&state=" + url.QueryEscape(p.state))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for valid callback, got %d", res.StatusCode)
	}

	io, _, _, _ := iostreams.Test()
	log := logger.New(io.ErrOut, logger.Info, false)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	token, err := waitForPKCEToken(ctx, io, log, "sess1", p)
	if err != nil {
		t.Fatal(err)
	}
	if token != "tok123" {
		t.Fatalf("expected tok123, got %q", token)
	}
}

func TestPKCELoginPastedCode(t *testing.T) {
	args := map[string]any{}

	p, err := newPKCELogin(args)
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()

	challenge, _ := args["code_challenge"].(string)

	ts := newRedeemServer(t, "sess2", "pasted-code", challenge, "tok456")
	defer ts.Close()
	fly.SetBaseURL(ts.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// blank and wrong lines are tolerated; the right code wins
	go p.readPastedCodes(ctx, strings.NewReader("\nwrong-code\npasted-code\n"))

	io, _, _, errOut := iostreams.Test()
	log := logger.New(io.ErrOut, logger.Info, false)

	token, err := waitForPKCEToken(ctx, io, log, "sess2", p)
	if err != nil {
		t.Fatal(err)
	}
	if token != "tok456" {
		t.Fatalf("expected tok456, got %q", token)
	}
	if !strings.Contains(errOut.String(), "invalid_code") {
		t.Fatalf("expected the wrong paste to be reported, got: %q", errOut.String())
	}
}
