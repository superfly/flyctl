package agent

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/flyctl"
)

func logDir() (string, error) {
	dir := filepath.Join(flyctl.ConfigDir(), "agent-logs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return dir, errors.Wrap(err, "failed to create log directory")
	}
	return dir, nil
}

func getLogFile(pid int) (*os.File, error) {
	dir, err := logDir()
	if err != nil {
		return nil, err
	}

	filePath := filepath.Join(dir, fmt.Sprintf("agent-%d.log", pid))
	logFile, err := os.Create(filePath)
	if err != nil {
		return nil, errors.Wrap(err, "can't create log file")
	}

	return logFile, nil
}

func currentLogFile() (*os.File, error) {
	return getLogFile(os.Getpid())
}

func InitAgentLogs() error {
	logFile, err := currentLogFile()
	if err != nil {
		return err
	}

	w := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(w)
	log.SetFlags(log.Lmsgprefix | log.LstdFlags)
	log.SetPrefix(fmt.Sprintf("[%d] ", os.Getpid()))

	go func() {
		if err := cleanLogFiles(); err != nil {
			log.Printf("failed to clean log files: %v", err)
		}
	}()

	return nil
}

func cleanLogFiles() error {
	logDir, err := logDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	for _, e := range entries {
		if i, _ := e.Info(); i.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(logDir, e.Name()))
		}
	}
	return nil
}
