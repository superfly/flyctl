package config

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/filemu"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/task"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/tp"
)

func TestRefreshDoesNotOverwriteLogin(t *testing.T) {
	for name, syncFirst := range map[string]bool{"refresh-before-sync": false, "sync-before-refresh": true} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yml")
			cached := tokens.ParseFromFile("fo1_old", path)
			before := cached.Copy()
			updated := tokens.ParseFromFile("fo1_refreshed_old", path)
			require.NoError(t, SetAccessToken(path, "fo1_login"))
			m := tokenMonitor{disk: cached.Copy()}
			if syncFirst {
				require.NoError(t, syncConfigTokens(&m, cached))
			}
			require.NoError(t, commitTokenRefresh(&m, cached, before, updated, path))
			// No watcher or subsequent session sync: a rejected refresh must
			// adopt disk credentials so the next refresh can make progress.
			require.Equal(t, "fo1_login", cached.All())
			disk, err := ReadAccessToken(path)
			require.NoError(t, err)
			require.Equal(t, "fo1_login", disk)
			require.NoError(t, commitTokenRefresh(&m, cached, cached.Copy(), tokens.ParseFromFile("fo1_next", path), path))
			disk, err = ReadAccessToken(path)
			require.NoError(t, err)
			require.Equal(t, "fo1_next", disk)
			require.Equal(t, disk, cached.All())
		})
	}
}

func TestRefreshCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	cached := tokens.ParseFromFile("fo1_old", path)
	before := cached.Copy()
	updated := tokens.ParseFromFile("fo1_refreshed", path)
	// Parsed comparison accepts the on-disk authorization scheme prefix.
	require.NoError(t, SetAccessToken(path, "Bearer fo1_old"))
	require.NoError(t, SetMetricsToken(path, "unrelated"))
	m := tokenMonitor{disk: cached.Copy()}
	require.NoError(t, commitTokenRefresh(&m, cached, before, updated, path))
	disk, err := ReadAccessToken(path)
	require.NoError(t, err)
	require.Equal(t, "fo1_refreshed", disk)
	require.Equal(t, disk, cached.All())
	values := map[string]any{}
	require.NoError(t, unmarshal(path, &values))
	require.Equal(t, "unrelated", values[MetricsTokenFileKey])

	// After persistence is disabled, unchanged disk must not undo memory refresh.
	updated = tokens.ParseFromFile("fo1_next", path)
	require.NoError(t, commitTokenRefresh(&m, cached, cached.Copy(), updated, ""))
	require.NoError(t, syncConfigTokens(&m, cached))
	require.Equal(t, "fo1_next", cached.All())
	// Preserve SetAccessToken's existing behavior when the file was removed.
	require.NoError(t, os.Remove(path))
	require.NoError(t, commitTokenRefresh(&m, cached, cached.Copy(), updated, path))
	disk, err = ReadAccessToken(path)
	require.NoError(t, err)
	require.Equal(t, "fo1_next", disk)
}

func TestSyncConfigTokensReadsUnderMonitorLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	cached := tokens.ParseFromFile("fo1_old", path)
	require.NoError(t, SetAccessToken(path, "fo1_old"))
	m := tokenMonitor{disk: cached.Copy()}
	m.Lock()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() { close(started); done <- syncConfigTokens(&m, cached) }()
	<-started
	// Keep the lock held while the pending sync would otherwise read old tokens.
	select {
	case err := <-done:
		m.Unlock()
		t.Fatalf("sync returned while monitor lock was held: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	err := SetAccessToken(path, "fo1_new")
	m.Unlock()
	require.NoError(t, err)
	require.NoError(t, <-done)
	require.Equal(t, "fo1_new", cached.All())
}

func tokenMonitorContext(t *testing.T) context.Context {
	t.Helper()
	t.Chdir(t.TempDir())
	ctx, cancel := context.WithCancel(logger.NewContext(context.Background(), logger.New(io.Discard, logger.Debug, false)))
	ctx = task.NewWithContext(ctx)
	task.FromContext(ctx).Start(ctx)
	t.Cleanup(func() { cancel(); task.FromContext(ctx).Shutdown() })

	return ctx
}

func TestInitialRefreshPreservesConcurrentLogin(t *testing.T) {
	ctx := tokenMonitorContext(t)
	path := filepath.Join(t.TempDir(), "config.yml")
	saved := make(chan error, 1)
	third := fakeThirdParty(t, func(w http.ResponseWriter, r *http.Request, party *tp.TP) {
		saved <- SetAccessToken(path, "fo1_new_login")
		party.RespondDischarge(w, r)
	})
	cached := tokens.ParseFromFile(fakeThirdPartyTokens(t, third, 1).All(), path)
	require.NoError(t, SetAccessToken(path, cached.All()))
	MonitorTokens(ctx, cached, nil)
	require.NoError(t, <-saved)
	disk, err := ReadAccessToken(path)
	require.NoError(t, err)
	require.Equal(t, "fo1_new_login", disk)
	require.Equal(t, disk, cached.All())
}

func TestSyncPreservesUnsavedDischarge(t *testing.T) {
	ctx := tokenMonitorContext(t)
	path := filepath.Join(t.TempDir(), "config.yml")
	locked := make(chan filemu.UnlockFunc, 1)
	third := fakeThirdParty(t, func(w http.ResponseWriter, r *http.Request, party *tp.TP) {
		unlock, err := filemu.Lock(ctx, lockPath())
		if err != nil {
			t.Error(err)
			http.Error(w, "lock failed", 500)
			return
		}
		locked <- unlock
		party.RespondDischarge(w, r)
	})
	cached := tokens.ParseFromFile(fakeThirdPartyTokens(t, third, 1).All(), path)
	original := cached.All()
	require.NoError(t, SetAccessToken(path, original))
	syncNow := MonitorTokens(ctx, cached, nil)
	select {
	case unlock := <-locked:
		require.NoError(t, unlock())
	case <-time.After(5 * time.Second):
		t.Fatal("discharge did not acquire the config lock")
	}
	disk, err := ReadAccessToken(path)
	require.NoError(t, err)
	require.True(t, disk == original, "setup must leave original credentials on disk")
	needs, err := flyio.DischargeClient().NeedsDischarge(cached.All())
	require.NoError(t, err)
	require.False(t, needs, "initial refresh must supply a real usable discharge")
	refreshed := cached.Copy()
	require.NoError(t, syncNow())
	require.True(t, refreshed.Equal(cached), "sync must preserve the unsaved discharge")
	// Unrelated config writes must not discard a discharge either.
	require.NoError(t, SetMetricsToken(path, "metrics"))
	require.NoError(t, syncNow())
	require.True(t, refreshed.Equal(cached))
	// Genuine login and logout changes still replace the in-memory credentials.
	require.NoError(t, SetAccessToken(path, "fo1_new_login"))
	require.NoError(t, syncNow())
	require.Equal(t, "fo1_new_login", cached.All())
	require.NoError(t, Clear(path))
	require.NoError(t, syncNow())
	require.Equal(t, "", cached.All())
}

// A foreground browser completion must survive a noninteractive monitor's
// partial save, without overriding a real login or logout during that work.
func TestInitialRefreshConcurrentBrowserCompletion(t *testing.T) {
	for _, replacement := range []string{"partial", "login", "logout", "cancel"} {
		t.Run(replacement, func(t *testing.T) {
			ctx, cancel := context.WithCancel(tokenMonitorContext(t))
			defer cancel()
			path := filepath.Join(t.TempDir(), "config.yml")
			direct := fakeThirdParty(t, func(w http.ResponseWriter, r *http.Request, p *tp.TP) { p.RespondDischarge(w, r) })
			browser := fakeThirdParty(t, func(w http.ResponseWriter, r *http.Request, p *tp.TP) { p.RespondUserInteractive(w, r) })
			original := fakeThirdPartyTokens(t, direct, 1).All() + "," + fakeThirdPartyTokens(t, browser, 2).All()
			foreground := tokens.ParseFromFile(original, path)
			require.NoError(t, SetAccessToken(path, original))
			calls := 0
			MonitorTokens(ctx, foreground, func(ctx context.Context, url string) error {
				calls++
				switch replacement {
				case "partial", "cancel":
					background := tokens.ParseFromFile(original, path)
					MonitorTokens(ctx, background, nil)
					needs, err := flyio.DischargeClient().NeedsDischarge(background.All())
					require.NoError(t, err)
					require.True(t, needs)
				case "login":
					require.NoError(t, SetAccessToken(path, "fo1_new_login"))
				case "logout":
					require.NoError(t, Clear(path))
				}
				if replacement == "cancel" {
					cancel()
					return ctx.Err()
				}

				return browser.DischargeUserInteractive(ctx, strings.TrimPrefix(url, "/user/"))
			})
			require.Equal(t, 1, calls)
			disk, err := ReadAccessToken(path)
			require.NoError(t, err)
			require.Equal(t, disk, foreground.All())
			switch replacement {
			case "partial":
				needs, err := flyio.DischargeClient().NeedsDischarge(disk)
				require.NoError(t, err)
				require.False(t, needs)
			case "login":
				require.Equal(t, "fo1_new_login", disk)
			case "logout":
				require.Empty(t, disk)
			case "cancel":
				needs, err := flyio.DischargeClient().NeedsDischarge(disk)
				require.NoError(t, err)
				require.True(t, needs)
			}
		})
	}
}

func TestCompletedDischargeConflicts(t *testing.T) {
	ctx := tokenMonitorContext(t)
	path := filepath.Join(t.TempDir(), "config.yml")
	third := fakeThirdParty(t, func(w http.ResponseWriter, r *http.Request, p *tp.TP) { p.RespondDischarge(w, r) })
	first := fakeThirdPartyTokens(t, third, 1)
	second := fakeThirdPartyTokens(t, third, 2)
	original := tokens.ParseFromFile(first.All()+","+second.All(), path)
	changed, err := refreshDischargeTokens(ctx, first, nil, time.Minute)
	require.NoError(t, err)
	require.True(t, changed)
	partial := tokens.ParseFromFile(first.All()+","+second.All(), path)
	complete := partial.Copy()
	changed, err = refreshDischargeTokens(ctx, complete, nil, time.Minute)
	require.NoError(t, err)
	require.True(t, changed)

	// The watcher may have adopted the partial result before refresh commits.
	require.NoError(t, SetAccessToken(path, partial.All()))
	cached := partial.Copy()
	m := tokenMonitor{disk: partial.Copy()}
	require.NoError(t, commitTokenRefresh(&m, cached, original, complete, path))
	require.True(t, cached.Equal(complete))
	// Reverse ordering: a stale partial refresh must adopt the completed result.
	cached = original.Copy()
	m = tokenMonitor{disk: original.Copy()}
	require.NoError(t, commitTokenRefresh(&m, cached, original, partial, path))
	require.True(t, cached.Equal(complete))
	disk, err := ReadAccessToken(path)
	require.NoError(t, err)
	require.Equal(t, complete.All(), disk)

	for name, current := range map[string]*tokens.Tokens{
		"new OAuth":                         partial.Copy().AddTokens("fo1_new"),
		"different permission for same org": tokens.ParseFromFile(fakeThirdPartyTokens(t, third, 1, 2).All(), path),
		"malformed":                         partial.Copy().AddTokens("fm2_bad"),
		"logout":                            tokens.ParseFromFile("", path),
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, SetAccessToken(path, current.All()))
			cached := original.Copy()
			m := tokenMonitor{disk: original.Copy()}
			require.NoError(t, commitTokenRefresh(&m, cached, original, complete, path))
			require.True(t, cached.Equal(current))
		})
	}
	require.False(t, preservesCompletedDischarge(original, partial, first), "pruning a permission must not look like completed authentication")
	require.False(t, preservesCompletedDischarge(original, partial, original), "partial must not replace partial")
	require.False(t, preservesCompletedDischarge(original, partial, complete.Copy().AddTokens("fo1_changed")), "changed refresh identity must not bypass CAS")
}
