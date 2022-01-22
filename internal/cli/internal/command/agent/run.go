package agent

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/agent/server"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/state"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/filemu"
	"github.com/superfly/flyctl/internal/logger"
)

func newRun() (cmd *cobra.Command) {
	const (
		short = "Run the Fly agent in the foreground"
		long  = short + "\n"
		usage = "run [log-file]"
	)

	cmd = command.New(usage, short, long, run)
	cmd.Aliases = []string{"daemon-start"}
	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Hidden = true

	return
}

func run(ctx context.Context) error {
	logPath := flag.FirstArg(ctx)
	logger, closeLogger, err := setupLogger(logPath)
	if err != nil {
		err = fmt.Errorf("failed setting up logger: %w", err)

		fmt.Fprintln(os.Stderr, err)
		return err
	}
	defer closeLogger()

	apiClient := client.FromContext(ctx)
	if !apiClient.Authenticated() {
		logger.Println(client.ErrNoAuthToken)

		return client.ErrNoAuthToken
	}

	unlock, err := lock(ctx, logger)
	if err != nil {
		return err
	}
	defer unlock()

	deleteOldLogs(ctx)

	opt := server.Options{
		Socket:     socketPath(ctx),
		Logger:     logger,
		Client:     apiClient.API(),
		Background: logPath != "",
		ConfigFile: state.ConfigFile(ctx),
	}

	return server.Run(ctx, opt)
}

func setupLogger(path string) (logger *log.Logger, close func(), err error) {
	var out io.Writer
	if path != "" {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return nil, nil, err
		}

		out = io.MultiWriter(os.Stdout, f)
		close = func() {
			_ = f.Sync()
			_ = f.Close()
		}
	} else {
		out = os.Stdout
		close = func() {}
	}

	logger = log.New(out, "srv ", log.Ldate|log.Lmicroseconds|log.Lmsgprefix)

	return
}

type errDupInstance struct {
	error
}

func (*errDupInstance) Error() string {
	return "another instance of the agent is already running"
}

func (*errDupInstance) Description() string {
	return "It looks like another instance of the agent is already running. Please stop it before starting a new one."
}

func (err *errDupInstance) Unwrap() error {
	return err.error
}

func lock(ctx context.Context, logger *log.Logger) (unlock filemu.UnlockFunc, err error) {
	path := filepath.Join(os.TempDir(), "fly-agent.lock")

	if unlock, err = filemu.Lock(ctx, path); err != nil {
		err = &errDupInstance{err}

		logger.Print(err)
	}

	return
}

func deleteOldLogs(ctx context.Context) {
	dir := filepath.Join(state.ConfigDirectory(ctx), "agent-logs")
	logger := logger.FromContext(ctx)

	entries, err := os.ReadDir(dir)
	if err != nil {
		logger.Warnf("failed reading agent log directory entries: %v", err)

		return
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	for _, e := range entries {
		if i, _ := e.Info(); i.ModTime().Before(cutoff) {
			p := filepath.Join(dir, e.Name())

			if err := os.Remove(p); err != nil {
				logger.Warnf("failed removing %s: %v", p, err)
			}
		}
	}
}
