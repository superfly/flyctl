// Package spritestate stores what the sprites commands remember between runs:
// the default organization and the Sprites API token minted for each
// organization. It is a separate package so that `fly auth logout` can clear
// it without importing the sprites commands.
package spritestate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

const fileName = "sprites.json"

// State is the content of the sprites state file.
type State struct {
	// DefaultOrg is the organization used when none is specified.
	DefaultOrg string `json:"default_org,omitempty"`

	// Tokens holds Sprites API tokens keyed by TokenKey.
	Tokens map[string]string `json:"tokens,omitempty"`
}

// Path returns the state file's location inside the flyctl config directory.
func Path(configDir string) string {
	return filepath.Join(configDir, fileName)
}

// TokenKey identifies a token by the API it is for and the organization it
// belongs to.
func TokenKey(apiURL, org string) string {
	return apiURL + "|" + org
}

// Load reads the state file. A missing file is an empty state.
func Load(configDir string) (*State, error) {
	st := &State{}

	data, err := os.ReadFile(Path(configDir))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return st, nil
	case err != nil:
		return nil, err
	}

	if err := json.Unmarshal(data, st); err != nil {
		// A corrupt state file only costs a new token, so start over rather
		// than block every sprites command.
		return &State{}, nil
	}

	return st, nil
}

// Save writes the state file with owner-only permissions, since it holds
// tokens. It replaces the file in one step, so a reader never sees it half
// written.
func Save(configDir string, st *State) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}

	// CreateTemp makes the file owner-only and gives each writer its own.
	tmp, err := os.CreateTemp(configDir, fileName+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck

	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}

	if err != nil {
		return err
	}

	return os.Rename(tmp.Name(), Path(configDir))
}

// How long to wait for the two locks. Saving state is quick, but several
// commands may be queued for it; minting a token involves a network request.
const (
	updateWait = 10 * time.Second
	mintWait   = time.Minute
)

// lock takes an exclusive lock on path, waiting up to wait for whoever holds
// it.
func lock(ctx context.Context, path string, wait time.Duration) (unlock func(), err error) {
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()

	mu := flock.New(path)

	locked, err := mu.TryLockContext(ctx, 10*time.Millisecond)
	switch {
	case err != nil:
		return nil, err
	case !locked:
		return nil, errors.New("the lock is held")
	}

	return func() { _ = mu.Unlock() }, nil
}

// Update applies fn to the current state and saves the result, holding a lock
// so that concurrent sprites commands do not drop each other's changes.
func Update(ctx context.Context, configDir string, fn func(*State)) error {
	unlock, err := lock(ctx, Path(configDir)+".lock", updateWait)
	if err != nil {
		return fmt.Errorf("timed out waiting for another sprites command to finish saving its state: %w", err)
	}
	defer unlock()

	st, err := Load(configDir)
	if err != nil {
		return err
	}

	fn(st)

	return Save(configDir, st)
}

// LockMint serializes minting tokens across processes. It is a separate lock
// from the one Update takes, so holding it does not block saving state.
func LockMint(ctx context.Context, configDir string) (unlock func(), err error) {
	unlock, err = lock(ctx, Path(configDir)+".mint.lock", mintWait)
	if err != nil {
		return nil, fmt.Errorf("timed out waiting for another sprites command to finish signing in: %w", err)
	}

	return unlock, nil
}

// Clear removes the state file. A missing file is not an error.
func Clear(configDir string) error {
	if err := os.Remove(Path(configDir)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	return nil
}
