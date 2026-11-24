package webauth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemovePendingLoginWhileOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending-login.json")
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	reader, err := openPendingLogin(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func TestReplacePendingLoginWhileOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending-login.json")
	if err := savePendingLogin(path, pendingLogin{ID: "old", Verifier: "first"}); err != nil {
		t.Fatal(err)
	}
	reader, err := openPendingLogin(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	if err := savePendingLogin(path, pendingLogin{ID: "new", Verifier: "second"}); err != nil {
		t.Fatal(err)
	}
	p, err := loadPendingLogin(path)
	if err != nil || p.ID != "new" {
		t.Fatalf("expected the replacement login, got ID %q, error %v", p.ID, err)
	}
}
