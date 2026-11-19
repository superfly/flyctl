package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/flyerr"
)

func TestSetupLogDirectoryPrunesLogs(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("FLY_CONFIG_DIR", tempDir)
	flyctl.InitConfig()

	logDir := filepath.Join(tempDir, "agent-logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatalf("failed creating logDir: %v", err)
	}

	// Create 15 log files with different mod times
	now := time.Now()
	for i := 0; i < 15; i++ {
		f, err := os.CreateTemp(logDir, "*.log")
		if err != nil {
			t.Fatalf("failed creating temp log: %v", err)
		}
		f.Close()
		modTime := now.Add(-time.Duration(i) * time.Minute)
		if err := os.Chtimes(f.Name(), modTime, modTime); err != nil {
			t.Fatalf("failed changing modTime: %v", err)
		}
	}

	// Create a non-log file in logDir
	nonLogPath := filepath.Join(logDir, "README.txt")
	if err := os.WriteFile(nonLogPath, []byte("preserve me"), 0o600); err != nil {
		t.Fatalf("failed creating non-log file: %v", err)
	}

	dir, err := setupLogDirectory()
	if err != nil {
		t.Fatalf("setupLogDirectory failed: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed reading logDir: %v", err)
	}

	// 10 log files + 1 non-log file = 11 total entries
	if len(entries) != 11 {
		t.Fatalf("expected 11 entries (10 retained log files + 1 non-log file), got %d", len(entries))
	}

	if _, err := os.Stat(nonLogPath); os.IsNotExist(err) {
		t.Fatalf("non-log file was improperly pruned: %v", nonLogPath)
	}
}

// A socket path over the limit can never be bound. The agent process refuses
// it; this side only sees the agent fail to come up, and must not report that
// to Sentry as a bug either.
func TestStartFailureWithOverlongSocketPathIsExpected(t *testing.T) {
	cause := errors.New("context deadline exceeded")

	long := &startError{error: expectedIfSocketPathTooLong(cause, "/"+strings.Repeat("a", MaxUnixSocketPath))}
	if !flyerr.IsExpected(long) {
		t.Fatal("start failure with an over-long socket path is not marked expected")
	}
	if got := long.Error(); got != "agent: failed to start" {
		t.Fatalf("message changed: %q", got)
	}

	short := &startError{error: expectedIfSocketPathTooLong(cause, "/tmp/fly-agent.sock")}
	if flyerr.IsExpected(short) {
		t.Fatal("an ordinary start failure must still be reported")
	}
}
