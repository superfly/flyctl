package spritestate

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadMissingFileIsEmpty(t *testing.T) {
	st, err := Load(t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, &State{}, st)
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()

	want := &State{
		DefaultOrg: "acme",
		Tokens:     map[string]string{TokenKey("https://api.sprites.dev", "acme"): "tok"},
	}
	require.NoError(t, Save(dir, want))

	got, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestSaveIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}

	dir := t.TempDir()
	require.NoError(t, Save(dir, &State{Tokens: map[string]string{"k": "secret"}}))

	info, err := os.Stat(Path(dir))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestLoadCorruptFileStartsOver(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(Path(dir), []byte("{not json"), 0o600))

	st, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, &State{}, st)
}

func TestClear(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Save(dir, &State{DefaultOrg: "acme"}))

	require.NoError(t, Clear(dir))
	assert.NoFileExists(t, Path(dir))

	// Clearing when there is nothing to clear is fine.
	require.NoError(t, Clear(dir))
}
