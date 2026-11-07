package webauth

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/superfly/flyctl/internal/state"
	"golang.org/x/sys/windows"
)

// holdExclusively opens path with no sharing, the way an antivirus scanner or
// another process can, so every other open of it fails as busy until the
// returned func closes it.
func holdExclusively(t *testing.T, path string) (release func()) {
	t.Helper()

	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}

	release = sync.OnceFunc(func() { _ = windows.CloseHandle(h) })
	t.Cleanup(release)

	return release
}

func TestBusyPendingLoginIsLeftForItsLogin(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	path := filepath.Join(dir, "pending-login.json")
	if err := savePendingLogin(path, pendingLogin{ID: "sess1", Verifier: "verifier1", ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	holdExclusively(t, path)

	ctx := state.WithConfigDirectory(context.Background(), dir)
	if _, _, err := RedeemPendingLogin(ctx, "code"); err == nil || !strings.Contains(err.Error(), "in use") {
		t.Fatalf("expected an in-use error, got %v", err)
	}
	removePendingLogin(path, "")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("a busy pending login was deleted: %v", err)
	}

	watch := &pendingWatch{path: path, configFile: filepath.Join(dir, "config.yml"), id: "sess1"}
	if _, done, err := watch.check(); done || err == nil {
		t.Fatalf("expected a busy read to keep waiting and report its error, got done=%v, %v", done, err)
	}
}

func TestPendingLoginReadOutlastsABriefHold(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending-login.json")
	if err := savePendingLogin(path, pendingLogin{ID: "sess1", Verifier: "verifier1", ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	release := holdExclusively(t, path)
	time.AfterFunc(50*time.Millisecond, release)

	if p, err := loadPendingLogin(path); err != nil || p.ID != "sess1" {
		t.Fatalf("expected the read to succeed once the hold ended, got %+v, %v", p, err)
	}
}
