package flapsutil

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// affinityDir holds the flaps app affinity tokens flyctl has been given
// recently, so that the next fly command touching the same app reaches the
// flaps instance that already has it, rather than one whose view of the app
// is still catching up.
//
// The layout is <config dir>/state/affinity/<url hash>/<app>: one file per
// app, holding only the token, with the token's expiry as its modification
// time.
var affinityDir = filepath.Join("state", "affinity")

// affinityPruneGrace is how long past its expiry a file is kept before a
// save removes it. A token is written with a future expiry, so a file this far
// past its own cannot have been replaced moments ago by another process; the
// grace keeps a prune from deleting a token saved while it was looking.
const affinityPruneGrace = time.Minute

// fileAffinityStore is a flaps.AffinityStore shared by every flyctl process of
// the same user.
//
// Tokens for different apps are independent, and of two processes saving one
// for the same app either may win, since any live token routes to an instance
// that has the app. So there is no lock: each save replaces one app's file by
// renaming a complete file over it, and a reader sees the old token or the new
// one. Each save also removes the files that expired a while ago, which keeps
// the directory to roughly the apps touched in the last couple of minutes.
//
// Failures of any kind leave the store behaving as empty: affinity only
// spares a request a stale read, it is never needed for one to succeed.
type fileAffinityStore struct {
	root string
}

func newFileAffinityStore(configDir string) *fileAffinityStore {
	return &fileAffinityStore{root: filepath.Join(configDir, affinityDir)}
}

// path returns the file holding appName's token at the flaps endpoint
// baseURL, or "" if appName cannot be a single safe path element.
func (s *fileAffinityStore) path(baseURL, appName string) string {
	if baseURL == "" || !safePathElem(appName) {
		return ""
	}

	return filepath.Join(s.root, affinityURLHash(baseURL), appName)
}

// affinityURLHash names the directory for a flaps endpoint: a short hash of
// its base URL, which keeps tokens from different endpoints apart without
// putting a URL's characters into a file name.
func affinityURLHash(baseURL string) string {
	sum := sha256.Sum256([]byte(baseURL))

	return hex.EncodeToString(sum[:8])
}

// safePathElem reports whether name can be used as one file name under the
// store. A leading "." is refused too: it keeps app files apart from the
// store's own temp files.
func safePathElem(name string) bool {
	return name != "" && !strings.HasPrefix(name, ".") && !strings.ContainsAny(name, `/\:`)
}

func (s *fileAffinityStore) LoadAffinity(baseURL, appName string) string {
	p := s.path(baseURL, appName)
	if p == "" {
		return ""
	}
	info, err := os.Stat(p)
	if err != nil || !time.Now().Before(info.ModTime()) {
		return ""
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}

func (s *fileAffinityStore) StoreAffinity(baseURL, appName, token string, expiresAt time.Time) {
	p := s.path(baseURL, appName)
	if p == "" || token == "" || !time.Now().Before(expiresAt) {
		return
	}

	s.prune()

	// A prune in another process may remove the endpoint directory between
	// creating it and writing into it; one retry covers that.
	for range 2 {
		if err := writeAffinityFile(p, token, expiresAt); err == nil {
			return
		}
	}
}

// writeAffinityFile replaces path with a file holding token whose
// modification time is expiresAt. The time is set before the rename, so no
// reader sees the new token with the wrong expiry.
func writeAffinityFile(path, token string, expiresAt time.Time) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()

	if _, err = f.WriteString(token); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Chtimes(tmp, expiresAt, expiresAt); err != nil {
		return err
	}

	return os.Rename(tmp, path)
}

// prune removes token files that expired more than affinityPruneGrace ago,
// temp files left by a process that died mid-save, and endpoint directories
// left empty.
func (s *fileAffinityStore) prune() {
	cutoff := time.Now().Add(-affinityPruneGrace)

	endpoints, err := os.ReadDir(s.root)
	if err != nil {
		return
	}
	for _, ep := range endpoints {
		if !ep.IsDir() {
			continue
		}
		dir := filepath.Join(s.root, ep.Name())
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			info, err := f.Info()
			if err != nil || f.IsDir() || !info.ModTime().Before(cutoff) {
				continue
			}
			_ = os.Remove(filepath.Join(dir, f.Name()))
		}
		// Fails, harmlessly, unless the directory is now empty.
		_ = os.Remove(dir)
	}
}
