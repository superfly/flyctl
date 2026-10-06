package helpers

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteFileAtomically_createsOwnerOnlyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret.json")

	if err := WriteFileAtomically(path, []byte("new\n"), 0o600); err != nil {
		t.Fatalf("WriteFileAtomically: %v", err)
	}

	if data, _ := os.ReadFile(path); string(data) != "new\n" {
		t.Errorf("unexpected contents %q", data)
	}
	requireMode(t, path, 0o600)
	requireNoTempFiles(t, filepath.Dir(path))
}

func TestWriteFileAtomically_replacesExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows refuses to rename over a file that another handle has open")
	}
	path := filepath.Join(t.TempDir(), "secret.json")
	if err := os.WriteFile(path, []byte("old and longer contents\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// A reader that opened the world-readable file before the rewrite must
	// never see the new contents: the rewrite has to land in a new inode.
	old, err := os.Open(path)
	if err != nil {
		t.Fatalf("open old: %v", err)
	}
	defer old.Close()

	if err := WriteFileAtomically(path, []byte("new\n"), 0o600); err != nil {
		t.Fatalf("WriteFileAtomically: %v", err)
	}

	if data, _ := os.ReadFile(path); string(data) != "new\n" {
		t.Errorf("unexpected contents %q", data)
	}
	requireMode(t, path, 0o600)
	requireNoTempFiles(t, filepath.Dir(path))

	seen, err := io.ReadAll(old)
	if err != nil {
		t.Fatalf("read through old handle: %v", err)
	}
	if string(seen) != "old and longer contents\n" {
		t.Errorf("old handle saw %q, want the original contents", seen)
	}
}

func TestWriteFileAtomically_keepsOldContentOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a read-only directory does not block file creation on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "secret.json")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// A read-only directory makes the temporary file impossible to create,
	// which is the closest portable stand-in for a write failing.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if err := WriteFileAtomically(path, []byte("new\n"), 0o600); err == nil {
		t.Fatal("expected an error from an unwritable directory")
	}

	if data, _ := os.ReadFile(path); string(data) != "old\n" {
		t.Errorf("a failed write must not touch the existing file, got %q", data)
	}
}

func requireMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("mode = %o, want %o", got, want)
	}
}

func requireNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file %s left behind", e.Name())
		}
	}
}
