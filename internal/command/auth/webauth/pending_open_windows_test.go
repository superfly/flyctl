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
