package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLatestChangeMissingConfigFile(t *testing.T) {
	at, err := latestChange(filepath.Join(t.TempDir(), "config.yml"))
	if err != nil {
		t.Fatalf("latestChange() error = %v, want nil", err)
	}

	if !at.IsZero() {
		t.Errorf("latestChange() = %v, want zero time", at)
	}
}

func TestLatestChangeExistingConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	at, err := latestChange(path)
	if err != nil {
		t.Fatalf("latestChange() error = %v, want nil", err)
	}

	if at.IsZero() {
		t.Error("latestChange() = zero time, want the file's mtime")
	}
}
