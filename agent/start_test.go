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
