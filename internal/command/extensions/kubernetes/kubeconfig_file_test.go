package kubernetes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteKubeconfigIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cluster.kubeconfig.yml")

	if err := writeKubeconfig(path, "apiVersion: v1\n"); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("new kubeconfig mode = %o, want 600", got)
	}
	if data, _ := os.ReadFile(path); string(data) != "apiVersion: v1\n" {
		t.Errorf("unexpected contents %q", data)
	}
}

func TestWriteKubeconfigNarrowsExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cluster.kubeconfig.yml")
	if err := os.WriteFile(path, []byte("old and longer contents\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := writeKubeconfig(path, "new\n"); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("existing kubeconfig mode = %o, want 600", got)
	}
	if data, _ := os.ReadFile(path); string(data) != "new\n" {
		t.Errorf("file not truncated, contents %q", data)
	}
}
