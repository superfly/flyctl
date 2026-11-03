package flapsutil

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/filemu"
)

// affinityFileName holds the flaps app affinity tokens flyctl has been given
// recently, so that the next fly command touching the same app reaches the
// flaps instance that already has it, rather than one whose view of the app
// is still catching up.
const affinityFileName = "flaps-affinity.json"

type affinityEntry struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// fileAffinityStore is a flaps.AffinityStore shared by every flyctl process of
// the same user. Tokens live for about a minute, so the file stays small:
// each write drops the entries that have expired.
//
// Readers need no lock, since the file is replaced atomically; writers take
// one so that two processes storing tokens for different apps both keep
// theirs. Failures of any kind leave the store behaving as empty: affinity
// only spares a request a stale read, it is never needed for one to succeed.
type fileAffinityStore struct {
	path string
}

func newFileAffinityStore(configDir string) *fileAffinityStore {
	return &fileAffinityStore{path: filepath.Join(configDir, affinityFileName)}
}

func affinityKey(baseURL, appName string) string {
	return baseURL + " " + appName
}

func (s *fileAffinityStore) read() map[string]affinityEntry {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil
	}
	var entries map[string]affinityEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil
	}
	return entries
}

func (s *fileAffinityStore) LoadAffinity(baseURL, appName string) string {
	e, ok := s.read()[affinityKey(baseURL, appName)]
	if !ok || !time.Now().Before(e.ExpiresAt) {
		return ""
	}
	return e.Token
}

func (s *fileAffinityStore) StoreAffinity(baseURL, appName, token string, expiresAt time.Time) {
	unlock, err := filemu.Lock(context.Background(), s.path+".lock")
	if err != nil {
		return
	}
	defer func() { _ = unlock() }()

	now := time.Now()
	entries := s.read()
	for k, e := range entries {
		if !now.Before(e.ExpiresAt) {
			delete(entries, k)
		}
	}
	if entries == nil {
		entries = map[string]affinityEntry{}
	}
	entries[affinityKey(baseURL, appName)] = affinityEntry{Token: token, ExpiresAt: expiresAt}

	data, err := json.Marshal(entries)
	if err != nil {
		return
	}
	_ = helpers.WriteFileAtomically(s.path, data, 0o600)
}
