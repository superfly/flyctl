package webauth

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/superfly/flyctl/iostreams"
)

// noRedirects is a client that reports a redirect instead of following it.
var noRedirects = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func TestPKCECallbackSendsANavigatingBrowserBack(t *testing.T) {
	p, err := newPKCELogin(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()

	callback := fmt.Sprintf("http://127.0.0.1:%d/callback?code=the-code&state=%s", p.port, url.QueryEscape(p.state))
	get := func(rawURL string) *http.Response {
		t.Helper()
		res, err := noRedirects.Get(rawURL)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()

		return res
	}

	// Until the server has named its origin there is nowhere to send it.
	if res := get(callback + "&return=1"); res.StatusCode != http.StatusOK {
		t.Fatalf("expected the plain page before the server URL is known, got %d", res.StatusCode)
	}

	p.setServerURL("https://fly.example/app/auth/cli/sess1?auto_redirect=1")

	// The page's background fetch carries no return=1 and keeps the page.
	if res := get(callback); res.StatusCode != http.StatusOK {
		t.Fatalf("expected the plain page for a fetch, got %d", res.StatusCode)
	}

	// A redirected browser goes back to the server's fixed page. Nothing in
	// the query can point it anywhere else.
	res := get(callback + "&return=1&connected=https://evil.example/&return_to=https://evil.example/")
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected a 303 for a redirected browser, got %d", res.StatusCode)
	}
	if got := res.Header.Get("Location"); got != "https://fly.example/app/auth/cli/connected" {
		t.Fatalf("redirected to %q", got)
	}

	// A wrong state is refused whatever else it asks for.
	if res := get(fmt.Sprintf("http://127.0.0.1:%d/callback?code=evil&state=wrong&return=1", p.port)); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for a bad state, got %d", res.StatusCode)
	}
}

func TestSetServerURLIgnoresWhatItCannotUse(t *testing.T) {
	for _, authURL := range []string{"", "not a url", "/app/auth/cli/sess1", "javascript:alert(1)", "ftp://fly.example/x"} {
		p := &pkceLogin{}
		p.setServerURL(authURL)
		if got := p.connectedURL.Load(); got != nil {
			t.Fatalf("setServerURL(%q) stored %q", authURL, *got)
		}
	}
}

func TestRunWebLoginMarksOnlyTheURLItOpens(t *testing.T) {
	remoteKeys := []string{"SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY", "CODESPACES", "REMOTE_CONTAINERS"}

	cases := []struct {
		name       string
		terminal   bool
		env        string
		wantMarker bool
	}{
		{name: "a terminal on this machine", terminal: true, wantMarker: true},
		{name: "over SSH", terminal: true, env: "SSH_CONNECTION"},
		{name: "in a codespace", terminal: true, env: "CODESPACES"},
		{name: "without a terminal", terminal: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, out, errOut := headlessContext(t)
			for _, key := range remoteKeys {
				t.Setenv(key, "")
			}
			if tc.env != "" {
				t.Setenv(tc.env, "1")
			}
			iostreams.FromContext(ctx).SetStdinTTY(tc.terminal)

			opened := make(chan string, 1)
			original := openBrowser
			openBrowser = func(u string) error {
				opened <- u

				return nil
			}
			t.Cleanup(func() { openBrowser = original })

			s := newLoginServer(t)
			done := make(chan waitResult, 1)
			go func() {
				token, err := RunWebLogin(ctx, false)
				done <- waitResult{token, err}
			}()

			s.approve(t, ctx)
			if r := awaitLogin(t, ctx, done); r.err != nil || r.token != "tok123" {
				t.Fatalf("expected token tok123, got %q, %v", r.token, r.err)
			}

			const bare = "https://fly.example/app/auth/cli/sess1"
			want := bare
			if tc.wantMarker {
				want = bare + "?auto_redirect=1"
			}
			if got := <-opened; got != want {
				t.Fatalf("opened %q, want %q", got, want)
			}
			// What a person might copy to another machine never has it.
			if printed := out.String() + errOut.String(); !strings.Contains(printed, bare) || strings.Contains(printed, autoRedirectParam) {
				t.Fatalf("printed URL should be the bare one: %q", printed)
			}
		})
	}
}
