package flapsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testBaseURL = "https://api.machines.dev"

func TestFileAffinityStoreSharedAcrossInstances(t *testing.T) {
	dir := t.TempDir()

	newFileAffinityStore(dir).StoreAffinity(testBaseURL, "app-a", "tok-a", time.Now().Add(time.Minute))

	// A second store over the same directory stands in for the next fly
	// process.
	s := newFileAffinityStore(dir)
	require.Equal(t, "tok-a", s.LoadAffinity(testBaseURL, "app-a"))
	require.Empty(t, s.LoadAffinity(testBaseURL, "app-b"))
	require.Empty(t, s.LoadAffinity("http://127.0.0.1:4280", "app-a"))

	p := filepath.Join(dir, "state", "affinity", affinityURLHash(testBaseURL), "app-a")
	require.Equal(t, p, s.path(testBaseURL, "app-a"))

	// Windows has no Unix permission bits to check.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(p)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestFileAffinityStorePaths(t *testing.T) {
	s := newFileAffinityStore("/cfg")
	prod := filepath.Join("/cfg", "state", "affinity", affinityURLHash(testBaseURL))

	require.Len(t, affinityURLHash(testBaseURL), 16)
	require.NotEqual(t, affinityURLHash(testBaseURL), affinityURLHash("http://127.0.0.1:4280"))

	for _, tc := range []struct {
		baseURL, app, want string
	}{
		{testBaseURL, "app-a", filepath.Join(prod, "app-a")},
		{"", "app-a", ""},
		{testBaseURL, "", ""},
		{testBaseURL, "..", ""},
		{testBaseURL, ".tmp-123", ""},
		{testBaseURL, "a/b", ""},
		{testBaseURL, `a\b`, ""},
	} {
		require.Equal(t, tc.want, s.path(tc.baseURL, tc.app), "path(%q, %q)", tc.baseURL, tc.app)
	}
}

func TestFileAffinityStoreIgnoresExpired(t *testing.T) {
	s := newFileAffinityStore(t.TempDir())

	s.StoreAffinity(testBaseURL, "app", "tok", time.Now().Add(-time.Second))
	require.Empty(t, s.LoadAffinity(testBaseURL, "app"))

	// Expired on disk since it was saved.
	require.NoError(t, writeAffinityFile(s.path(testBaseURL, "app"), "tok", time.Now().Add(-time.Second)))
	require.Empty(t, s.LoadAffinity(testBaseURL, "app"))
}

func TestFileAffinityStoreReplacesToken(t *testing.T) {
	s := newFileAffinityStore(t.TempDir())

	s.StoreAffinity(testBaseURL, "app", "first", time.Now().Add(time.Minute))
	s.StoreAffinity(testBaseURL, "app", "second", time.Now().Add(time.Minute))
	require.Equal(t, "second", s.LoadAffinity(testBaseURL, "app"))
}

func TestFileAffinityStorePrunes(t *testing.T) {
	s := newFileAffinityStore(t.TempDir())
	longAgo := time.Now().Add(-affinityPruneGrace - time.Minute)

	stale := s.path(testBaseURL, "stale")
	require.NoError(t, writeAffinityFile(stale, "tok", longAgo))
	// Just expired: kept, in case another process is replacing it right now.
	recent := s.path(testBaseURL, "recent")
	require.NoError(t, writeAffinityFile(recent, "tok", time.Now().Add(-time.Second)))
	// A temp file from a process that died mid-save.
	orphan := filepath.Join(filepath.Dir(stale), ".tmp-orphan")
	require.NoError(t, os.WriteFile(orphan, []byte("tok"), 0o600))
	require.NoError(t, os.Chtimes(orphan, longAgo, longAgo))
	// An endpoint whose only token is long gone.
	other := s.path("http://127.0.0.1:4280", "app")
	require.NoError(t, writeAffinityFile(other, "tok", longAgo))

	s.StoreAffinity(testBaseURL, "fresh", "tok", time.Now().Add(time.Minute))

	require.NoFileExists(t, stale)
	require.NoFileExists(t, orphan)
	require.FileExists(t, recent)
	require.NoDirExists(t, filepath.Dir(other))
	require.Equal(t, "tok", s.LoadAffinity(testBaseURL, "fresh"))
}

func TestFileAffinityStoreConcurrentWriters(t *testing.T) {
	dir := t.TempDir()
	apps := []string{"a", "b", "c", "d", "e", "f", "g", "h"}

	var wg sync.WaitGroup
	for _, app := range apps {
		for i := range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				newFileAffinityStore(dir).StoreAffinity(testBaseURL, app, fmt.Sprintf("tok-%s-%d", app, i), time.Now().Add(time.Minute))
			}()
		}
	}
	wg.Wait()

	s := newFileAffinityStore(dir)
	for _, app := range apps {
		// Any of the writers may win; none may leave a mix or nothing.
		require.Regexp(t, `^tok-`+app+`-[0-3]$`, s.LoadAffinity(testBaseURL, app))
	}

	entries, err := os.ReadDir(filepath.Join(dir, affinityDir, affinityURLHash(testBaseURL)))
	require.NoError(t, err)
	require.Len(t, entries, len(apps), "temp files left behind")
}

func TestFileAffinityStoreReadDuringWrites(t *testing.T) {
	s := newFileAffinityStore(t.TempDir())
	s.StoreAffinity(testBaseURL, "app", "tok-0", time.Now().Add(time.Minute))

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 200 {
			s.StoreAffinity(testBaseURL, "app", fmt.Sprintf("tok-%d", i), time.Now().Add(time.Minute))
		}
	}()

	for {
		select {
		case <-done:
			return
		default:
		}
		got := s.LoadAffinity(testBaseURL, "app")
		// On Windows a rename can briefly make the file unreadable, which
		// the store reports as no token.
		if got == "" && runtime.GOOS == "windows" {
			continue
		}
		require.Regexp(t, `^tok-\d+$`, got)
	}
}
