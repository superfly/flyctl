package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/state"
	"github.com/superfly/flyctl/internal/filemu"
	"github.com/superfly/flyctl/pkg/agent/server"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newRun() (cmd *cobra.Command) {
	const (
		short = "Run the Fly agent in the foreground"
		long  = short + "\n"
	)

	cmd = command.New("run", short, long, run,
		command.RequireSession,
	)

	cmd.Hidden = true
	cmd.Args = cobra.MaximumNArgs(1)

	return
}

func run(ctx context.Context) error {
	unlock, err := lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()

	// ensure the logs dir exist
	logger, close, err := setupLogger(ctx)
	if err != nil {
		return err
	}
	defer close()

	l, err := bind(ctx)
	if err != nil {
		return err
	}
	// no need to close l as server.Serve will

	// write the pid file

	return server.Serve(ctx, l, logger, flag.FirstArg(ctx) != "")
}

func lock(ctx context.Context) (unlock filemu.UnlockFunc, err error) {
	path := filepath.Join(os.TempDir(), "agent.lock")

	if unlock, err = filemu.Lock(ctx, path); err != nil {
		err = fmt.Errorf("failed locking %s: %w. is another agent process running?", path, err)
	}

	return
}

func setupLogger(ctx context.Context) (logger *log.Logger, close func() error, err error) {
	dir := filepath.Join(state.ConfigDirectory(ctx), "agent-logs")

	if err = os.MkdirAll(dir, 0700); err != nil {
		err = fmt.Errorf("failed creating logs directory at %s: %w", dir, err)

		return
	}

	var entries []fs.DirEntry
	if entries, err = os.ReadDir(dir); err != nil {
		err = fmt.Errorf("failed reading logs directory entries at %s: %w", dir, err)

		return
	}

	now := time.Now()

	cutoff := now.Add(-24 * time.Hour)
	for _, e := range entries {
		if i, _ := e.Info(); i.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}

	pattern := fmt.Sprintf("%d-*.log", os.Getpid())

	var file *os.File
	if file, err = os.CreateTemp(dir, pattern); err != nil {
		err = fmt.Errorf("failed creating log file: %w", err)

		return
	}

	close = file.Close

	out := io.MultiWriter(iostreams.FromContext(ctx).Out, file)
	logger = log.New(out, "", log.LstdFlags)

	return
}

func bind(ctx context.Context) (l net.Listener, err error) {
	path := pathToSocket(ctx)

	switch err = os.RemoveAll(path); {
	case errors.Is(err, fs.ErrNotExist):
		err = nil
	case err != nil:
		err = fmt.Errorf("failed unlinking previous socket file at %s: %w", path, err)

		return
	}

	if l, err = net.Listen("unix", path); err != nil {
		err = fmt.Errorf("failed binding on %s: %w", path, err)
	}

	return
}

func writePID(ctx context.Context) error {
	path := pathToPID(ctx)

	data := strconv.Itoa(os.Getpid())

	return os.WriteFile(path, []byte(data), 0600)
}
