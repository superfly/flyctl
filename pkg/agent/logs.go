package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/superfly/flyctl/flyctl"
)

func logDir() (dir string, err error) {
	dir = filepath.Join(flyctl.ConfigDir(), "agent-logs")

	if err = os.MkdirAll(dir, 0700); err != nil {
		err = fmt.Errorf("failed to create log directory: %w", err)
	}

	return
}

func CleanLogFiles() error {
	dir, err := logDir()
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	for _, e := range entries {
		if i, _ := e.Info(); i.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}

	return nil
}
