package agent

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/client"
)

func newDaemonStart() *cobra.Command {
	const (
		short = "Run the Fly agent as a service (manually)"
		long  = short + "\n"
	)

	return command.New("daemon-start", short, long, runDaemonStart,
		command.RequireSession,
	)
}

func runDaemonStart(ctx context.Context) error {
	if err := agent.InitAgentLogs(); err != nil {
		return fmt.Errorf("failed initializing agent logs: %w", err)
	}

	io := iostreams.FromContext(ctx)

	if err := agent.StopRunningAgent(); err != nil {
		fmt.Fprintf(io.ErrOut, "failed stopping existing agent: %v\n", err)
	}

	if err := agent.CreatePidFile(); err != nil {
		fmt.Fprintf(io.ErrOut, "failed creating pid file: %v\n", err)
	}

	defer fmt.Fprintln(io.ErrOut, "QUIT")
	defer func() {
		if err := agent.RemovePidFile(); err != nil {
			fmt.Fprintf(io.ErrOut, "failed removing pid file: %v\n", err)
		}
	}()

	api := client.FromContext(ctx).API()

	agent, err := agent.DefaultServer(api, !io.IsInteractive())
	if err != nil {
		return fmt.Errorf("failed starting daemon: %w", err)
	}

	fmt.Fprintf(io.ErrOut, "OK %d\n", os.Getpid())

	agent.Serve()

	go func() {
		<-ctx.Done()

		agent.Stop()
	}()

	agent.Wait()

	return nil
}
