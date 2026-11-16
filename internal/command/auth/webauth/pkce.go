package webauth

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/iostreams"
)

// pkceCode is a completion-code attempt plus how it arrived: codes delivered
// by the loopback callback leave the terminal cursor parked on the prompt
// line, so the caller clears it; pasted codes already ended with the user's
// Enter and stay visible.
type pkceCode struct {
	code     string
	fromHTTP bool
	// redirected is set for a code the server handed over by redirecting the
	// browser here, which the listener then sent on to the connected page:
	// that browser never showed the code.
	redirected bool
}

type pkceLogin struct {
	verifier string
	state    string
	codes    chan pkceCode
	server   *http.Server
	port     int

	// connectedURL is the page a browser that navigated to the callback is
	// sent on to. The listener is already serving when the server names its
	// origin, hence the atomic.
	connectedURL atomic.Pointer[string]

	// mu guards held and spare. held is set while a code from the callback
	// is with the terminal. A callback that arrives meanwhile, such as the
	// code page's "Send to terminal" after approving again, is kept as spare
	// and tried if the held code fails: that page can't tell whether we took
	// its code, so dropping it would leave the user thinking it was sent.
	mu    sync.Mutex
	held  bool
	spare *pkceCode
}

// hold claims the callback slot for a code. When a code is already held it
// keeps this one as the spare instead and reports false.
func (p *pkceLogin) hold(code pkceCode) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.held {
		p.spare = &code

		return false
	}
	p.held = true

	return true
}

// release is called when the held code was not used. It returns the spare
// to try next, which keeps the slot held, or frees the slot.
func (p *pkceLogin) release() *pkceCode {
	p.mu.Lock()
	defer p.mu.Unlock()

	next := p.spare
	p.spare = nil
	p.held = next != nil

	return next
}

// connectedPath is the server's page for a login the browser handed over by
// redirect. It is fixed here: the callback's query never chooses where the
// browser goes next.
const connectedPath = "/app/auth/cli/connected"

// autoRedirectParam marks the login URL flyctl opens in a browser itself. The
// server answers an approval on that URL by redirecting the browser to the
// loopback callback, which only works when the browser is on this machine.
const autoRedirectParam = "auto_redirect"

// setServerURL records where the browser goes after a redirected callback:
// the login URL's scheme and host, with connectedPath. The session id lets
// that page watch for this process redeeming the code, so it only says
// "connected" once that has happened. It reports whether it stored one; a
// login URL it can't use leaves nowhere to send the browser.
func (p *pkceLogin) setServerURL(authURL, sessionID string) bool {
	u, err := url.Parse(authURL)
	if err != nil || u.Host == "" {
		return false
	}
	// Plain http only for a local development server: the redirect carries
	// the session id.
	if u.Scheme != "https" && (u.Scheme != "http" || !isLoopbackHost(u.Hostname())) {
		return false
	}

	connected := &url.URL{Scheme: u.Scheme, Host: u.Host, Path: connectedPath}
	if sessionID != "" {
		connected.RawQuery = url.Values{"session": {sessionID}}.Encode()
	}
	s := connected.String()
	p.connectedURL.Store(&s)

	return true
}

// redirectURL returns the login URL to open asking for the redirect, and
// arms the redirect back, or "" when it can't do both. A marker without
// somewhere to send the browser afterwards would strand it on our bare page.
func (p *pkceLogin) redirectURL(authURL, sessionID string) string {
	marked, added := withAutoRedirect(authURL)
	if !added || !p.setServerURL(authURL, sessionID) {
		return ""
	}

	return marked
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}

// withAutoRedirect appends autoRedirectParam to the login URL, leaving the
// rest of the URL exactly as the server issued it. It reports whether it
// added the marker: a URL that already carries the parameter is left alone.
func withAutoRedirect(authURL string) (string, bool) {
	u, err := url.Parse(authURL)
	if err != nil || u.Query().Has(autoRedirectParam) {
		return authURL, false
	}

	// Splice the marker in as text, before any fragment, rather than
	// rebuilding the URL, which would re-escape its path and fragment.
	base, fragment, hasFragment := strings.Cut(authURL, "#")
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
		if strings.HasSuffix(base, "?") || strings.HasSuffix(base, "&") {
			sep = ""
		}
	}
	marked := base + sep + autoRedirectParam + "=1"
	if hasFragment {
		marked += "#" + fragment
	}

	return marked, true
}

func newPKCELogin(args map[string]any) (*pkceLogin, error) {
	verifier, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	state, err := randomToken(16)
	if err != nil {
		return nil, err
	}

	p := &pkceLogin{
		verifier: verifier,
		state:    state,
		codes:    make(chan pkceCode, 1),
	}

	// Binding the loopback listener is best-effort: without it the login is
	// paste-only, and the caller refuses to start when there is no terminal
	// to paste into either.
	if l, err := net.Listen("tcp", "127.0.0.1:0"); err == nil {
		p.port = l.Addr().(*net.TCPAddr).Port
		p.serve(l)
	}

	challenge := sha256.Sum256([]byte(verifier))
	args["code_challenge"] = base64.RawURLEncoding.EncodeToString(challenge[:])
	args["code_challenge_method"] = "S256"
	args["state"] = state
	if p.port != 0 {
		args["redirect_port"] = p.port
	}

	return p, nil
}

func randomToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (p *pkceLogin) serve(l net.Listener) {
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		// The approval page notifies us with a cross-origin fetch; answer
		// preflights (including Chrome's legacy private-network one) permissively.
		// The response carries no secrets.
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)

			return
		}

		q := r.URL.Query()
		if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(p.state)) != 1 || q.Get("code") == "" {
			http.Error(w, "invalid callback", http.StatusBadRequest)

			return
		}

		// The server adds return=1 when it redirected the browser here, as
		// opposed to its page fetching this in the background.
		connected := p.connectedURL.Load()
		redirected := connected != nil && q.Get("return") == "1"

		code := pkceCode{code: q.Get("code"), fromHTTP: true, redirected: redirected}
		if !p.hold(code) {
			// The terminal is already trying a code; this one is kept in case
			// that fails. Don't send this browser to the connected page,
			// which would wait on a code that may never be tried.
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, "<html><body><p>Your terminal is already signing in with a code from this login. Return to your terminal to see how it went.</p></body></html>")

			return
		}

		select {
		case p.codes <- code:
		case <-r.Context().Done():
			if next := p.release(); next != nil {
				go p.deliver(r.Context(), *next)
			}

			return
		}

		// Send a redirected browser back to a page of the server's rather
		// than leave it on a bare one of ours.
		if redirected {
			http.Redirect(w, r, *connected, http.StatusSeeOther)

			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<html><body><p>Signed in! You can close this tab and return to your terminal.</p></body></html>")
	})

	p.server = &http.Server{Handler: mux}
	go p.server.Serve(l) //nolint:errcheck // closed via Shutdown
}

// deliver hands a kept code to the waiting loop.
func (p *pkceLogin) deliver(ctx context.Context, code pkceCode) {
	select {
	case p.codes <- code:
	case <-ctx.Done():
	}
}

func (p *pkceLogin) close() {
	if p.server != nil {
		ctx, cancel := context.WithTimeoutCause(context.Background(), time.Second, fmt.Errorf("shutting down login callback server: %w", context.DeadlineExceeded))
		defer cancel()
		_ = p.server.Shutdown(ctx)
	}
}

// readPastedCodes forwards non-empty stdin lines as completion-code attempts.
func (p *pkceLogin) readPastedCodes(ctx context.Context, in io.Reader) {
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		code := strings.TrimSpace(scanner.Text())
		if code == "" {
			continue
		}
		select {
		case p.codes <- pkceCode{code: code}:
		case <-ctx.Done():
			return
		}
	}
}

// waitForPKCEToken blocks until a completion code arrives and redeems it.
// With acceptPaste the code may also be typed on stdin; without it (no
// terminal) the loopback callback is the only source. With watch it also
// stops once another process finishes or replaces the login.
func waitForPKCEToken(parent context.Context, io *iostreams.IOStreams, log *logger.Logger, id string, p *pkceLogin, acceptPaste bool, watch *pendingWatch) (string, error) {
	ctx, cancel := context.WithTimeoutCause(parent, loginTimeout, fmt.Errorf("waiting for PKCE login: %w", context.DeadlineExceeded))
	defer cancel()
	defer p.close()

	prompt := ""
	if acceptPaste {
		prompt = "paste code here if prompted > "
		fmt.Fprint(io.Out, prompt)
		go p.readPastedCodes(ctx, io.In)
	}

	ticker := time.NewTicker(pendingCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			if watch == nil {
				continue
			}
			if token, done, err := watch.check(); done {
				if acceptPaste {
					fmt.Fprint(io.Out, "\r\x1b[K")
				}
				log.Debugf("pending login ended in another process: %v", err)
				if err == nil && token != "" {
					metrics.SetLoginMethod(metrics.LoginMethodOtherProcess)
				}

				return token, err
			}
		case attempt := <-p.codes:
			// An HTTP-delivered code leaves the cursor on the prompt line;
			// erase it so the flow's output starts clean. Pasted codes ended
			// with the user's Enter and remain visible above.
			if attempt.fromHTTP && acceptPaste {
				fmt.Fprint(io.Out, "\r\x1b[K")
			}

			session, err := fly.RedeemCLISessionToken(ctx, id, attempt.code, p.verifier)
			switch {
			case err == nil && session.AccessToken != "":
				log.Debug("redeemed access token.")
				method := metrics.LoginMethodPaste
				switch {
				case attempt.redirected:
					method = metrics.LoginMethodRedirect
				case attempt.fromHTTP:
					method = metrics.LoginMethodCallback
				}
				metrics.SetLoginMethod(method)

				return session.AccessToken, nil
			case err == nil:
				return "", errors.New("failed to log in, please try again")
			case errors.Is(err, fly.ErrNotFound):
				return "", errors.New("login session expired, please try again")
			default:
				log.Debugf("failed redeeming code: %v", err)
				if !acceptPaste {
					// The callback comes only once, and nobody can paste
					// another code without a terminal: stop with a way
					// forward instead of waiting out the timeout.
					return "", fmt.Errorf("failed to redeem the login code (%w). Run `fly auth login` again", err)
				}
				if attempt.fromHTTP {
					// Try a code that arrived meanwhile, or let the next
					// callback hand one over.
					if next := p.release(); next != nil {
						go p.deliver(ctx, *next)
					}
				}
				if attempt.redirected {
					// The browser was handed the code without showing it. The
					// server redirects only the first approval, so approving
					// again shows a code to paste.
					fmt.Fprintf(io.ErrOut, "Signing in with the code from the browser failed (%v).\nGo back to the browser tab and approve again to get a code to paste here.\n", err)
				} else {
					fmt.Fprintf(io.ErrOut, "That code didn't work (%v).\n", err)
				}
				fmt.Fprint(io.Out, prompt)
			}
		}
	}
}
