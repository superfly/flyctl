package webauth

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
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
	// get returns the status and Location of a request, closing its body.
	get := func(rawURL string) (int, string) {
		t.Helper()
		res, err := noRedirects.Get(rawURL)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		return res.StatusCode, res.Header.Get("Location")
	}

	// take drains the delivered code and lets the listener accept another,
	// as a failed redemption would.
	take := func() {
		t.Helper()
		select {
		case <-p.codes:
		default:
			t.Fatal("the callback delivered no code")
		}
		p.release()
	}

	// Until the server has named its origin there is nowhere to send it.
	if status, _ := get(callback + "&return=1"); status != http.StatusOK {
		t.Fatalf("expected the plain page before the server URL is known, got %d", status)
	}
	take()

	p.setServerURL("https://fly.example/app/auth/cli/sess1?auto_redirect=1", "sess1")

	// The page's background fetch carries no return=1 and keeps the page.
	if status, _ := get(callback); status != http.StatusOK {
		t.Fatalf("expected the plain page for a fetch, got %d", status)
	}
	take()

	// A redirected browser goes back to the server's fixed page. Nothing in
	// the query can point it anywhere else.
	status, got := get(callback + "&return=1&connected=https://evil.example/&return_to=https://evil.example/")
	if status != http.StatusSeeOther {
		t.Fatalf("expected a 303 for a redirected browser, got %d", status)
	}
	if got != "https://fly.example/app/auth/cli/connected?session=sess1" {
		t.Fatalf("redirected to %q", got)
	}
	take()

	// A wrong state is refused whatever else it asks for.
	if status, _ := get(fmt.Sprintf("http://127.0.0.1:%d/callback?code=evil&state=wrong&return=1", p.port)); status != http.StatusBadRequest {
		t.Fatalf("expected 400 for a bad state, got %d", status)
	}
}

func TestSetServerURLIgnoresWhatItCannotUse(t *testing.T) {
	for _, authURL := range []string{"", "not a url", "/app/auth/cli/sess1", "javascript:alert(1)", "ftp://fly.example/x", "http://fly.example/app/auth/cli/sess1"} {
		p := &pkceLogin{}
		p.setServerURL(authURL, "sess1")
		if got := p.connectedURL.Load(); got != nil {
			t.Fatalf("setServerURL(%q) stored %q", authURL, *got)
		}
	}
}

func TestRunWebLoginMarksOnlyTheURLItOpens(t *testing.T) {
	cases := []struct {
		name       string
		terminal   bool
		env        string
		wantMarker bool
	}{
		{name: "a terminal on this machine", terminal: true, wantMarker: true},
		{name: "over SSH", terminal: true, env: "SSH_CONNECTION"},
		{name: "in a codespace", terminal: true, env: "CODESPACES"},
		{name: "in Gitpod", terminal: true, env: "GITPOD_WORKSPACE_ID"},
		{name: "in WSL", terminal: true, env: "WSL_DISTRO_NAME"},
		{name: "with a forwarding BROWSER", terminal: true, env: "BROWSER"},
		{name: "without a terminal", terminal: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, out, errOut := headlessContext(t)
			localSession(t)
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

func TestWithAutoRedirectKeepsTheIssuedURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://fly.example/app/auth/cli/sess1", "https://fly.example/app/auth/cli/sess1?auto_redirect=1"},
		{"https://fly.example/app/auth/cli/sess1?z=2&a=1;b+c%20d", "https://fly.example/app/auth/cli/sess1?z=2&a=1;b+c%20d&auto_redirect=1"},
		{"https://fly.example/app/auth/cli/sess1?", "https://fly.example/app/auth/cli/sess1?auto_redirect=1"},
		// Path and fragment come back byte for byte, not re-escaped.
		{"https://fly.example/app/auth/cli/a b#x y", "https://fly.example/app/auth/cli/a b?auto_redirect=1#x y"},
		{"https://fly.example/app/auth/cli/sess1?a=1#frag", "https://fly.example/app/auth/cli/sess1?a=1&auto_redirect=1#frag"},
	}
	for _, tc := range cases {
		got, added := withAutoRedirect(tc.in)
		if got != tc.want || !added {
			t.Errorf("withAutoRedirect(%q) = %q, %v; want %q, true", tc.in, got, added, tc.want)
		}
	}

	// A URL that already says something about the redirect is left alone,
	// and the caller is told so.
	const issued = "https://fly.example/app/auth/cli/sess1?auto_redirect=0"
	if got, added := withAutoRedirect(issued); got != issued || added {
		t.Errorf("withAutoRedirect(%q) = %q, %v; want it unchanged", issued, got, added)
	}
}

// localSession clears every signal remoteSession reads, so the run looks like
// a terminal on the browser's machine. The original values come back after
// the test.
func localSession(t *testing.T) {
	t.Helper()

	for _, key := range append([]string{"BROWSER", "WAYLAND_DISPLAY"}, remoteSessionEnv...) {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
	t.Setenv("DISPLAY", ":0")
}

func TestRemoteSessionOnLinuxNeedsADisplay(t *testing.T) {
	localSession(t)
	original := goos
	goos = "linux"
	t.Cleanup(func() { goos = original })

	if remote, why := remoteSession(); remote {
		t.Fatalf("with a DISPLAY: remote (%s)", why)
	}

	os.Unsetenv("DISPLAY")
	if remote, _ := remoteSession(); !remote {
		t.Fatal("without DISPLAY or WAYLAND_DISPLAY: expected remote")
	}

	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	if remote, why := remoteSession(); remote {
		t.Fatalf("with WAYLAND_DISPLAY: remote (%s)", why)
	}
}

func TestPKCECallbackKeepsACodeThatArrivesWhileOneIsHeld(t *testing.T) {
	p, err := newPKCELogin(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()
	p.setServerURL("https://fly.example/app/auth/cli/sess1", "sess1")

	callback := func(code, extra string) string {
		return fmt.Sprintf("http://127.0.0.1:%d/callback?code=%s&state=%s%s", p.port, code, url.QueryEscape(p.state), extra)
	}
	get := func(rawURL string) int {
		t.Helper()
		res, err := noRedirects.Get(rawURL)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()

		return res.StatusCode
	}

	if status := get(callback("first", "&return=1")); status != http.StatusSeeOther {
		t.Fatalf("first callback: expected 303, got %d", status)
	}
	if got := <-p.codes; got.code != "first" || !got.redirected {
		t.Fatalf("first callback delivered %+v", got)
	}

	// The code page's "Send to terminal" while the first code is being
	// redeemed: kept, not handed over yet, and not sent to the connected
	// page, which would wait on a code that may never be tried.
	if status := get(callback("second", "")); status != http.StatusOK {
		t.Fatalf("callback while a code is held: expected 200, got %d", status)
	}
	select {
	case got := <-p.codes:
		t.Fatalf("a held login took another code: %+v", got)
	default:
	}

	// The first code failed: the kept one is next, and the slot stays held.
	next := p.release()
	if next == nil || next.code != "second" || next.redirected {
		t.Fatalf("expected the kept code next, got %+v", next)
	}

	// That failed too, with nothing kept: the next callback is taken.
	if next := p.release(); next != nil {
		t.Fatalf("expected nothing kept, got %+v", next)
	}
	if status := get(callback("third", "&return=1")); status != http.StatusSeeOther {
		t.Fatalf("callback after a failure: expected 303, got %d", status)
	}
	if got := <-p.codes; got.code != "third" {
		t.Fatalf("callback after a failure delivered %+v", got)
	}
}

func TestRedirectURLNeedsSomewhereToSendTheBrowser(t *testing.T) {
	for authURL, want := range map[string]string{
		"https://fly.example/app/auth/cli/sess1": "https://fly.example/app/auth/cli/sess1?auto_redirect=1",
		// The redirect back can't be armed, so don't ask for the redirect.
		"http://staging.fly.example/app/auth/cli/sess1": "",
		// The server already said something about it.
		"https://fly.example/app/auth/cli/sess1?auto_redirect=0": "",
	} {
		p := &pkceLogin{}
		if got := p.redirectURL(authURL, "sess1"); got != want {
			t.Errorf("redirectURL(%q) = %q, want %q", authURL, got, want)
		}
		if armed := p.connectedURL.Load() != nil; armed != (want != "") {
			t.Errorf("redirectURL(%q): redirect back armed = %v", authURL, armed)
		}
	}
}

func TestRunWebLoginRecoversFromAFailedRedirectedCode(t *testing.T) {
	ctx, _, _ := headlessContext(t)
	localSession(t)
	iostreams.FromContext(ctx).SetStdinTTY(true)

	opened := make(chan struct{})
	original := openBrowser
	openBrowser = func(string) error {
		close(opened)

		return nil
	}
	t.Cleanup(func() { openBrowser = original })

	s := newLoginServer(t)
	done := make(chan waitResult, 1)
	go func() {
		token, err := RunWebLogin(ctx, false)
		done <- waitResult{token, err}
	}()
	// The redirect back is armed by the time the browser opens.
	select {
	case <-opened:
	case <-ctx.Done():
		t.Fatal("the browser was never opened")
	}

	s.mu.Lock()
	callback := func(code string) string {
		return fmt.Sprintf("http://127.0.0.1:%v/callback?code=%s&state=%s", s.args["redirect_port"], code, url.QueryEscape(fmt.Sprint(s.args["state"])))
	}
	s.mu.Unlock()

	status := func(rawURL string) int {
		t.Helper()
		res, err := noRedirects.Get(rawURL)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()

		return res.StatusCode
	}

	// The server rejects this code, as if redeeming it had failed.
	if got := status(callback("wrong") + "&return=1"); got != http.StatusSeeOther {
		t.Fatalf("first callback: expected 303, got %d", got)
	}

	// Approving again shows the code page, whose "Send to terminal" may
	// arrive before or after the failure is processed: either way the
	// terminal signs in with it.
	if got := status(callback("the-code")); got != http.StatusOK {
		t.Fatalf("second callback: expected 200, got %d", got)
	}

	if r := awaitLogin(t, ctx, done); r.err != nil || r.token != "tok123" {
		t.Fatalf("expected token tok123, got %q, %v", r.token, r.err)
	}
}

func TestSetServerURLAllowsPlainHTTPOnlyLocally(t *testing.T) {
	for _, authURL := range []string{"http://localhost:4000/app/auth/cli/sess1", "http://127.0.0.1:4000/app/auth/cli/sess1"} {
		p := &pkceLogin{}
		p.setServerURL(authURL, "sess1")
		if p.connectedURL.Load() == nil {
			t.Fatalf("setServerURL(%q) stored nothing", authURL)
		}
	}
}
