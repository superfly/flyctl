package agent

import (
	"context"
	"fmt"
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

func (fe *forkError) Unwrap() error { return fe.error }

func StartDaemon(ctx context.Context) (*Client, error) {
	logFile, err := prepareLogFile()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(os.Args[0], "agent", "run", logFile)
	cmd.Env = append(os.Environ(), "FLY_NO_UPDATE_CHECK=1")
	setCommandFlags(cmd)

	if err := cmd.Start(); err != nil {
		sentry.CaptureException(err)

		return nil, fmt.Errorf("failed starting agent process: %w", err)
	}

	if logger := logger.MaybeFromContext(ctx); logger != nil {
		logger.Infof("started agent process (PID: %d)", cmd.Process.Pid)
	}

	switch client, err := waitForClient(ctx); {
	case err == nil:
		return client, nil
	case ctx.Err() != nil:
		return nil, ctx.Err()
	default:
		return nil, errFailedToStart(logFile)
	}
}

type errFailedToStart string

func (errFailedToStart) Error() string {
	return "agent: failed to start"
}

func (err errFailedToStart) Description() string {
	return "The agent failed to start. You may review the log file here: " + string(err)
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

func prepareLogFile() (path string, err error) {
	path = filepath.Join(flyctl.ConfigDir(), "agent-logs")

	if err = os.MkdirAll(path, 0700); err != nil {
		err = fmt.Errorf("failed creating log directory at %s: %w", path, err)

		return
	}

	var f *os.File
	if f, err = os.CreateTemp(path, "*.log"); err != nil {
		err = fmt.Errorf("failed creating log file at %s: %w", f.Name(), err)
	} else if err = f.Close(); err != nil {
		err = fmt.Errorf("failed closing log file at %s: %w", f.Name(), err)
	} else {
		path = f.Name()
	}

	return
}
