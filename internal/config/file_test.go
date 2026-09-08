package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteFileAtomicallyReplacesContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")

	require.NoError(t, os.WriteFile(path, []byte("access_token: old\n"), 0o600))
	require.NoError(t, writeFileAtomically(path, []byte("access_token: new\n"), 0o600))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "access_token: new\n", string(got))

	if runtime.GOOS != "windows" {
		// Windows has no Unix permission bits; Go reports 0666 for every file.
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "no temporary file should be left behind")
}

func TestWriteFileAtomicallyKeepsOldContentOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a read-only directory does not block file creation on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("access_token: old\n"), 0o600))

	// A read-only directory makes the temporary file impossible to create,
	// which is the closest portable stand-in for a write failing.
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	require.Error(t, writeFileAtomically(path, []byte("access_token: new\n"), 0o600))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "access_token: old\n", string(got), "a failed write must not touch the existing file")
}

func TestSetAccessTokenRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")

	require.NoError(t, SetAccessToken(path, "fo1_token"))

	token, err := ReadAccessToken(path)
	require.NoError(t, err)
	require.Equal(t, "fo1_token", token)
}
