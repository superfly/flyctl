package cmd

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/agent/server"

	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"
)

func newAgentCommand(client *client.Client) *Command {
	cmd := BuildCommandKS(nil, nil, docstrings.Get("agent"), client, requireSession)
	cmd.Hidden = true

	_ = BuildCommandKS(cmd,
		runFlyAgentDaemonStart,
		docstrings.Get("agent.daemon-start"),
		client,
		requireSession)

	_ = BuildCommandKS(cmd,
		runFlyAgentStart,
		docstrings.Get("agent.start"),
		client,
		requireSession)

	_ = BuildCommandKS(cmd,
		runFlyAgentStart,
		docstrings.Get("agent.restart"),
		client,
		requireSession)

	_ = BuildCommandKS(cmd,
		runFlyAgentStop,
		docstrings.Get("agent.stop"),
		client,
		requireSession)

	_ = BuildCommandKS(cmd,
		runFlyAgentPing,
		docstrings.Get("agent.ping"),
		client,
		requireSession)

	return cmd
}

func runFlyAgentDaemonStart(cc *cmdctx.CmdContext) error {
	logPath := agentLogPath(cc)
	logger, closeLogger, err := setupAgentLogger(logPath)
	if err != nil {
		err = fmt.Errorf("failed setting up agent logger: %w", err)

		logger.Print(err)
		return err
	}
	defer closeLogger()

	ctx := cc.Command.Context()

	if err := agent.CleanLogFiles(); err != nil {
		err = fmt.Errorf("failed to clean agent logs: %w", err)

		logger.Print(err)
		return err
	}

	if err := agent.StopRunningAgent(); err != nil {
		err = fmt.Errorf("failed to stop existing agent: %w", err)

		logger.Print(err)
		return err
	}

	if err := agent.CreatePidFile(); err != nil {
		err = fmt.Errorf("failed to create pid file: %w", err)

		logger.Print(err)
		return err
	}
	defer agent.RemovePidFile()

	server.Run(ctx, logger, cc.Client.API(), logPath != "")

	return nil
}

func agentLogPath(cc *cmdctx.CmdContext) string {
	if len(cc.Args) > 0 {
		return cc.Args[0]
	}

	return ""
}

func setupAgentLogger(path string) (logger *log.Logger, close func(), err error) {

	var out io.Writer
	if path != "" {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return nil, nil, err
		}

		out = io.MultiWriter(os.Stdout, f)
		close = func() { _ = f.Close() }
	} else {
		out = os.Stdout
		close = func() {}
	}

	logger = log.New(out, fmt.Sprintf("[%d] ", os.Getpid()), log.LstdFlags|log.Lmsgprefix)

	return
}

func runFlyAgentStart(cc *cmdctx.CmdContext) error {
	api := cc.Client.API()
	ctx := context.Background()

	if client, err := agent.DefaultClient(ctx); err == nil {
		_ = client.Kill(ctx)
	}

	if _, err := agent.Establish(ctx, api); err != nil {
		return fmt.Errorf("failed to start agent: %w", err)
	}

	return nil
}

func runFlyAgentStop(cc *cmdctx.CmdContext) error {
	ctx := context.Background()

	c, err := dialAgent(ctx)
	if err != nil {
		return err
	}

	if err := c.Kill(ctx); err != nil {
		return fmt.Errorf("can't kill agent: %w", err)
	}

	return nil
}

func runFlyAgentPing(cc *cmdctx.CmdContext) error {
	ctx := context.Background()

	c, err := dialAgent(ctx)
	if err != nil {
		return err
	}

	res, err := c.Ping(ctx)
	if err == nil {
		return fmt.Errorf("can't ping agent: %w", err)
	}

	cc.WriteJSON(res)

	return nil
}

func dialAgent(ctx context.Context) (client *agent.Client, err error) {
	if client, err = agent.DefaultClient(ctx); err != nil {
		err = fmt.Errorf("can't connect to agent: %w", err)
	}

	return
}
