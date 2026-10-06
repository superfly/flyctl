package webauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

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
