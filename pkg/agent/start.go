package agent

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/azazeal/pause"

	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/sentry"
)

type forkError struct{ error }

func (fe forkError) Unwrap() error { return fe.error }

func StartDaemon(ctx context.Context) (*Client, error) {
	logFile, err := createLogFile()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(os.Args[0], "agent", "run", logFile)
	cmd.Env = append(os.Environ(), "FLY_NO_UPDATE_CHECK=1")
	setCommandFlags(cmd)

	if err := cmd.Start(); err != nil {
		err = forkError{err}
		sentry.CaptureException(err)

		return nil, fmt.Errorf("failed starting agent process: %w", err)
	}

	if logger := logger.MaybeFromContext(ctx); logger != nil {
		logger.Infof("started agent process (pid: %d, log: %s)", cmd.Process.Pid, logFile)
	}

	switch client, err := waitForClient(ctx); {
	case err == nil:
		return client, nil
	case ctx.Err() != nil:
		return nil, ctx.Err()
	default:
		err = &startError{
			error:   err,
			logFile: logFile,
		}

		if log := readLogFile(logFile); log != "" {
			sentry.CaptureException(err, sentry.WithExtra("log", log))
		} else {
			sentry.CaptureException(err)
		}

		return nil, err
	}
}

func readLogFile(path string) (log string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	const limit = 10 * 1 << 10
	if len(data) > limit {
		data = data[:limit]
	}

	return string(data)
}

type startError struct {
	error
	logFile string
}

func (*startError) Error() string {
	return "agent: failed to start"
}

func (se *startError) Unwrap() error { return se.error }

func (se *startError) Description() string {
	return fmt.Sprintf("The agent failed to start. You may review the log file here: %s", se.logFile)
}

func waitForClient(ctx context.Context) (*Client, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	for ctx.Err() == nil {
		pause.For(ctx, 100*time.Millisecond)

		c, err := DefaultClient(ctx)
		if err == nil {
			return c, nil
		}
	}

	return nil, ctx.Err()
}

func createLogFile() (path string, err error) {
	var dir string
	if dir, err = setupLogDirectory(); err != nil {
		return
	}

	var f *os.File
	if f, err = os.CreateTemp(dir, "*.log"); err != nil {
		err = fmt.Errorf("failed creating log file: %w", err)
	} else if err = f.Close(); err != nil {
		err = fmt.Errorf("failed closing log file: %w", err)
	} else {
		path = f.Name()
	}

	return
}

func setupLogDirectory() (dir string, err error) {
	dir = filepath.Join(flyctl.ConfigDir(), "agent-logs")

	if err = os.MkdirAll(dir, 0700); err != nil {
		err = fmt.Errorf("failed creating agent log directory at %s: %w", dir, err)

		return
	}

	var entries []fs.DirEntry
	if entries, err = os.ReadDir(dir); err != nil {
		err = fmt.Errorf("failed reading agent log directory entries: %v", err)

		return
	}

	cutoff := time.Now().AddDate(0, 0, -1)

	for _, entry := range entries {
		switch inf, e := entry.Info(); {
		case e != nil:
			continue
		case !inf.Mode().IsRegular():
			continue
		case inf.ModTime().Before(cutoff):
			p := filepath.Join(dir, inf.Name())

			_ = os.Remove(p)
		}
	}

	return
}
