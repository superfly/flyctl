package agent

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/agent/server"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/filemu"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/state"
)

func newRun() (cmd *cobra.Command) {
	const (
		short = "Run the Fly agent in the foreground"
		long  = short + "\n"
	)

	cmd = command.New("run", short, long, run)

	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Aliases = []string{"daemon-start"}

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

type dupInstanceError struct{}

func (*dupInstanceError) Error() string {
	return "another instance of the agent is already running"
}

func (*dupInstanceError) Description() string {
	return "It looks like another instance of the agent is already running. Please stop it before starting a new one."
}

var (
	lockPath       = filepath.Join(os.TempDir(), "flyctl.agent.lock")
	errDupInstance = new(dupInstanceError)
)

func lock(ctx context.Context, logger *log.Logger) (unlock filemu.UnlockFunc, err error) {
	switch unlock, err = filemu.Lock(ctx, lockPath); {
	case err == nil:
		break // all done
	case ctx.Err() != nil:
		err = ctx.Err() // parent canceled or deadlined
	default:
		err = errDupInstance

		logger.Print(err)
	}

	return
}
