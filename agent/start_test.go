package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/superfly/flyctl/flyctl"
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

	dir, err := setupLogDirectory()
	if err != nil {
		t.Fatalf("setupLogDirectory failed: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed reading logDir: %v", err)
	}

	if len(entries) > 10 {
		t.Fatalf("expected at most 10 log files retained, got %d", len(entries))
	}
}
