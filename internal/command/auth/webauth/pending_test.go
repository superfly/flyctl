package webauth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPendingLoginFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending-login.json")
	want := pendingLogin{ID: "sess1", Verifier: "verifier1", ExpiresAt: time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)}

	if err := savePendingLogin(path, want); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("pending login is readable by others: mode %o", mode)
	}

	got, err := loadPendingLogin(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.Verifier != want.Verifier || !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Fatalf("loaded %+v, saved %+v", got, want)
	}

	// A new login replaces the old one.
	if err := savePendingLogin(path, pendingLogin{ID: "sess2", Verifier: "verifier2", ExpiresAt: want.ExpiresAt}); err != nil {
		t.Fatal(err)
	}
	if got, _ := loadPendingLogin(path); got.ID != "sess2" {
		t.Fatalf("expected the newer login sess2, got %q", got.ID)
	}
}

func TestLoadPendingLoginErrors(t *testing.T) {
	dir := t.TempDir()

	if _, err := loadPendingLogin(filepath.Join(dir, "missing.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist for a missing file, got %v", err)
	}

	for name, content := range map[string]string{
		"corrupt":    "{not json",
		"incomplete": `{"id":"sess1"}`,
	} {
		path := filepath.Join(dir, name+".json")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadPendingLogin(path); err == nil || errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s file: expected a read error, got %v", name, err)
		}
	}
}

func TestRemovePendingLogin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending-login.json")
	p := pendingLogin{ID: "sess1", Verifier: "verifier1", ExpiresAt: time.Now().Add(time.Minute)}

	// A file holding another session stays: a newer login replaced ours.
	if err := savePendingLogin(path, p); err != nil {
		t.Fatal(err)
	}
	removePendingLogin(path, "sess-other")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("removed a pending login that belongs to another session: %v", err)
	}

	removePendingLogin(path, "sess1")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected our pending login removed, stat: %v", err)
	}

	// Nothing can finish an unreadable file, so it goes too.
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	removePendingLogin(path, "sess1")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected the unreadable file removed, stat: %v", err)
	}
}
