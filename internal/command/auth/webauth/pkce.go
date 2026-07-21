package webauth

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/iostreams"
)

// pkceCode is a completion-code attempt plus how it arrived: codes delivered
// by the loopback callback leave the terminal cursor parked on the prompt
// line, so the caller clears it; pasted codes already ended with the user's
// Enter and stay visible.
type pkceCode struct {
	code     string
	fromHTTP bool
}

type pkceLogin struct {
	verifier string
	state    string
	codes    chan pkceCode
	server   *http.Server
	port     int
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
	// paste-only, which is fine because the caller guarantees a TTY.
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
	var once sync.Once

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
		if q.Get("state") != p.state || q.Get("code") == "" {
			http.Error(w, "invalid callback", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<html><body><p>Signed in! You can close this tab and return to your terminal.</p></body></html>")

		once.Do(func() { p.codes <- pkceCode{code: q.Get("code"), fromHTTP: true} })
	})

	p.server = &http.Server{Handler: mux}
	go p.server.Serve(l) //nolint:errcheck // closed via Shutdown
}

func (p *pkceLogin) close() {
	if p.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
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

func waitForPKCEToken(parent context.Context, io *iostreams.IOStreams, log *logger.Logger, id string, p *pkceLogin) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Minute)
	defer cancel()
	defer p.close()

	prompt := "paste code here if prompted > "
	fmt.Fprint(io.Out, prompt)
	go p.readPastedCodes(ctx, io.In)

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case attempt := <-p.codes:
			// An HTTP-delivered code leaves the cursor on the prompt line;
			// erase it so the flow's output starts clean. Pasted codes ended
			// with the user's Enter and remain visible above.
			if attempt.fromHTTP {
				fmt.Fprint(io.Out, "\r\x1b[K")
			}

			session, err := fly.RedeemCLISessionToken(ctx, id, attempt.code, p.verifier)
			switch {
			case err == nil && session.AccessToken != "":
				log.Debug("redeemed access token.")
				return session.AccessToken, nil
			case err == nil:
				return "", errors.New("failed to log in, please try again")
			case errors.Is(err, fly.ErrNotFound):
				return "", errors.New("login session expired, please try again")
			default:
				log.Debugf("failed redeeming code: %v", err)
				fmt.Fprintf(io.ErrOut, "That code didn't work (%v).\n", err)
				fmt.Fprint(io.Out, prompt)
			}
		}
	}
}
