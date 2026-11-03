package flapsutil

import (
	"os"
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
	require.Empty(t, s.LoadAffinity("https://other.example", "app-a"))

	// Windows has no Unix permission bits to check.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(s.path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestFileAffinityStoreDropsExpired(t *testing.T) {
	s := newFileAffinityStore(t.TempDir())

	s.StoreAffinity(testBaseURL, "old", "tok-old", time.Now().Add(-time.Second))
	require.Empty(t, s.LoadAffinity(testBaseURL, "old"))

	s.StoreAffinity(testBaseURL, "new", "tok-new", time.Now().Add(time.Minute))
	require.NotContains(t, s.read(), affinityKey(testBaseURL, "old"))
	require.Equal(t, "tok-new", s.LoadAffinity(testBaseURL, "new"))
}

func TestFileAffinityStoreReplacesToken(t *testing.T) {
	s := newFileAffinityStore(t.TempDir())

	s.StoreAffinity(testBaseURL, "app", "first", time.Now().Add(time.Minute))
	s.StoreAffinity(testBaseURL, "app", "second", time.Now().Add(time.Minute))
	require.Equal(t, "second", s.LoadAffinity(testBaseURL, "app"))
}

func TestFileAffinityStoreConcurrentWritersKeepAllApps(t *testing.T) {
	dir := t.TempDir()
	apps := []string{"a", "b", "c", "d", "e", "f", "g", "h"}

	var wg sync.WaitGroup
	for _, app := range apps {
		wg.Add(1)
		go func() {
			defer wg.Done()
			newFileAffinityStore(dir).StoreAffinity(testBaseURL, app, "tok-"+app, time.Now().Add(time.Minute))
		}()
	}
	wg.Wait()

	s := newFileAffinityStore(dir)
	for _, app := range apps {
		require.Equal(t, "tok-"+app, s.LoadAffinity(testBaseURL, app))
	}
}

func TestFileAffinityStoreToleratesBadFile(t *testing.T) {
	s := newFileAffinityStore(t.TempDir())
	require.NoError(t, os.WriteFile(s.path, []byte("not json"), 0o600))

	require.Empty(t, s.LoadAffinity(testBaseURL, "app"))
	s.StoreAffinity(testBaseURL, "app", "tok", time.Now().Add(time.Minute))
	require.Equal(t, "tok", s.LoadAffinity(testBaseURL, "app"))
}
