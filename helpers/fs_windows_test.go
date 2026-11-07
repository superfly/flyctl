package helpers

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// A rename over a file another process has open fails on Windows until it is
// closed; WriteFileAtomically waits that out.
func TestWriteFileAtomicallyOutlastsABriefHold(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	time.AfterFunc(50*time.Millisecond, func() { _ = windows.CloseHandle(h) })

	if err := WriteFileAtomically(path, []byte("new\n"), 0o600); err != nil {
		t.Fatalf("WriteFileAtomically: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new\n" {
		t.Fatalf("got %q, want the new contents", got)
	}
}

// A read-only file stays read-only, so a rename over it fails at once rather
// than after the busy retries.
func TestWriteFileAtomicallyFailsFastOnAReadOnlyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte("old\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) }) // so the temp dir can be removed

	start := time.Now()
	if err := WriteFileAtomically(path, []byte("new\n"), 0o600); err == nil {
		t.Fatal("expected the rename over a read-only file to fail")
	}
	if elapsed := time.Since(start); elapsed >= fileBusyDelay {
		t.Fatalf("took %v: a read-only file was retried as busy", elapsed)
	}
}
