package webauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/state"
)

// pendingLogin is a PKCE login saved while it waits for approval, so that
// `fly auth login --code` can finish it from another process: after the
// waiting one was killed, or when the browser is on another machine and its
// callback can't reach this one.
type pendingLogin struct {
	ID        string    `json:"id"`
	Verifier  string    `json:"verifier"`
	ExpiresAt time.Time `json:"expires_at"`
}

func pendingLoginPath(ctx context.Context) string {
	return filepath.Join(state.ConfigDirectory(ctx), "pending-login.json")
}

// savePendingLogin writes p to path, replacing any earlier pending login.
// Together with a completion code the verifier yields a token, so the file is
// as private as config.yml.
func savePendingLogin(path string, p pendingLogin) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}

	return helpers.WriteFileAtomically(path, data, 0o600)
}

// loadPendingLogin reads the pending login at path. The error wraps
// os.ErrNotExist when there is none.
func loadPendingLogin(path string) (pendingLogin, error) {
	var p pendingLogin

	data, err := os.ReadFile(path)
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return p, fmt.Errorf("reading %s: %w", path, err)
	}
	if p.ID == "" || p.Verifier == "" {
		return p, fmt.Errorf("reading %s: incomplete pending login", path)
	}

	return p, nil
}

// removePendingLogin deletes the pending login at path unless it belongs to
// another session, which means a newer login replaced it. An unreadable file
// is deleted too: nothing can finish it.
func removePendingLogin(path, id string) {
	p, err := loadPendingLogin(path)
	if errors.Is(err, os.ErrNotExist) || (err == nil && p.ID != id) {
		return
	}

	_ = os.Remove(path)
}

var errLoginExpired = errors.New("that login has expired. Run `fly auth login` to start a new one")

// RedeemPendingLogin finishes the login an earlier `fly auth login` left
// waiting, with the one-time code its approval page shows, and returns the
// token. Call finish once the token is saved: it removes the pending login,
// and a login still waiting in another process takes that as its cue to stop.
func RedeemPendingLogin(ctx context.Context, code string) (token string, finish func(), err error) {
	path := pendingLoginPath(ctx)

	p, err := loadPendingLogin(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "", nil, errors.New("no login is waiting for a code here. Run `fly auth login` first, then use the code its approval page shows")
	case err != nil:
		_ = os.Remove(path)

		return "", nil, fmt.Errorf("the saved login could not be read (%w). Run `fly auth login` to start a new one", err)
	case time.Now().After(p.ExpiresAt):
		_ = os.Remove(path)

		return "", nil, errLoginExpired
	}

	session, err := fly.RedeemCLISessionToken(ctx, p.ID, strings.TrimSpace(code), p.Verifier)
	switch {
	case errors.Is(err, fly.ErrNotFound):
		removePendingLogin(path, p.ID)

		return "", nil, errLoginExpired
	case err != nil && strings.Contains(err.Error(), "authorization_pending"):
		return "", nil, errors.New("this login hasn't been approved yet. Approve it in the browser, then use the code the page shows")
	case err != nil:
		return "", nil, fmt.Errorf("that code didn't work (%w). Copy the whole code from the approval page and try again", err)
	case session.AccessToken == "":
		removePendingLogin(path, p.ID)

		return "", nil, errors.New("failed to log in, please try again")
	}

	return session.AccessToken, func() { removePendingLogin(path, p.ID) }, nil
}
