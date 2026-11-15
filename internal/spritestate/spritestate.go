// Package spritestate stores what the sprites commands remember between runs:
// the default organization and the Sprites API token minted for each
// organization. It is a separate package so that `fly auth logout` can clear
// it without importing the sprites commands.
package spritestate

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/superfly/flyctl/internal/filemu"
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

// Update applies fn to the current state and saves the result, holding a lock
// so that concurrent sprites commands do not drop each other's changes.
func Update(ctx context.Context, configDir string, fn func(*State)) error {
	unlock, err := filemu.Lock(ctx, Path(configDir)+".lock")
	if err != nil {
		return err
	}
	defer unlock() //nolint:errcheck

	st, err := Load(configDir)
	if err != nil {
		return err
	}

	fn(st)

	return Save(configDir, st)
}

// Clear removes the state file. A missing file is not an error.
func Clear(configDir string) error {
	if err := os.Remove(Path(configDir)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	return nil
}
