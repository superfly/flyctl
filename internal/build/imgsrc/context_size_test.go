package imgsrc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSizedFile writes a file of exactly size bytes at rel within dir,
// creating parent directories as needed.
func writeSizedFile(t *testing.T, dir, rel string, size int) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, make([]byte, size), 0o644))
}

func entryByName(entries []buildContextEntry, name string) (buildContextEntry, bool) {
	for _, e := range entries {
		if e.name == name {
			return e, true
		}
	}

	return buildContextEntry{}, false
}

func TestAnalyzeBuildContext_TotalsAndLargestPaths(t *testing.T) {
	dir := t.TempDir()
	writeSizedFile(t, dir, "Dockerfile", 100)
	writeSizedFile(t, dir, "app/main.go", 500)
	writeSizedFile(t, dir, ".cache/blob.bin", 4000)
	writeSizedFile(t, dir, ".cache/nested/more.bin", 1000)
	writeSizedFile(t, dir, "keep.txt", 250)

	stats, err := analyzeBuildContext(dir, filepath.Join(dir, "Dockerfile"), "")
	require.NoError(t, err)

	// Without a .dockerignore, everything but fly.toml (absent here) is included.
	assert.Equal(t, int64(100+500+4000+1000+250), stats.totalBytes)
	assert.Equal(t, 5, stats.fileCount)

	// Largest top-level path is .cache, aggregating its nested files, and is
	// reported as a directory.
	require.NotEmpty(t, stats.entries)
	assert.Equal(t, ".cache", stats.entries[0].name)
	assert.Equal(t, int64(5000), stats.entries[0].bytes)
	assert.True(t, stats.entries[0].isDir)
	assert.Equal(t, ".cache/", stats.entries[0].displayName())

	app, ok := entryByName(stats.entries, "app")
	require.True(t, ok)
	assert.Equal(t, int64(500), app.bytes)
	assert.True(t, app.isDir)

	keep, ok := entryByName(stats.entries, "keep.txt")
	require.True(t, ok)
	assert.False(t, keep.isDir)
}

func TestAnalyzeBuildContext_HonorsDockerignore(t *testing.T) {
	dir := t.TempDir()
	writeSizedFile(t, dir, "Dockerfile", 100)
	writeSizedFile(t, dir, "app/main.go", 500)
	writeSizedFile(t, dir, ".cache/blob.bin", 9000)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte(".cache\n"), 0o644))

	stats, err := analyzeBuildContext(dir, filepath.Join(dir, "Dockerfile"), "")
	require.NoError(t, err)

	_, found := entryByName(stats.entries, ".cache")
	assert.False(t, found, "ignored .cache should not be counted")
	assert.Less(t, stats.totalBytes, int64(9000), "ignored bytes should be excluded")

	app, ok := entryByName(stats.entries, "app")
	require.True(t, ok)
	assert.Equal(t, int64(500), app.bytes)
}

func TestAnalyzeBuildContext_NegatedReinclude(t *testing.T) {
	dir := t.TempDir()
	writeSizedFile(t, dir, "Dockerfile", 100)
	writeSizedFile(t, dir, "logs/big.log", 8000)
	writeSizedFile(t, dir, "logs/keep.log", 30)

	ignore := "logs\n!logs/keep.log\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte(ignore), 0o644))

	stats, err := analyzeBuildContext(dir, filepath.Join(dir, "Dockerfile"), "")
	require.NoError(t, err)

	// big.log is ignored, but the negated keep.log is walked back in.
	logs, ok := entryByName(stats.entries, "logs")
	require.True(t, ok, "re-included logs/keep.log should be counted")
	assert.Equal(t, int64(30), logs.bytes)
}

func TestResolveBuildContextWarnBytes(t *testing.T) {
	const mib = int64(1 << 20) // matches units.MiB
	defaultBytes := 100 * mib

	cases := []struct {
		name      string
		flagValue string // "" means the flag was not set
		envValue  string
		wantBytes int64
		wantOn    bool
	}{
		{"default", "", "", defaultBytes, true},
		{"env plain number is MB", "", "250", 250 * mib, true},
		{"env human size", "", "2gb", 2048 * mib, true},
		{"env disables", "", "0", 0, false},
		{"env invalid falls back to default", "", "not-a-size", defaultBytes, true},
		{"flag plain number is MB", "300", "", 300 * mib, true},
		{"flag human size", "1gb", "", 1024 * mib, true},
		{"flag disables", "0", "", 0, false},
		{"flag beats env", "300", "200", 300 * mib, true},
		{"flag disable beats env", "0", "500", 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotBytes, gotOn := resolveBuildContextWarnBytes(tc.flagValue, tc.envValue)
			assert.Equal(t, tc.wantOn, gotOn)
			if tc.wantOn {
				assert.Equal(t, tc.wantBytes, gotBytes)
			}
		})
	}
}
