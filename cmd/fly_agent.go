package cmd

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/agent"
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
	ctx := cc.Command.Context()

	if err := agent.InitAgentLogs(); err != nil {
		return err
	}

	if err := agent.StopRunningAgent(); err != nil {
		log.Printf("failed to stop existing agent: %v", err)
	}
	if err := agent.CreatePidFile(); err != nil {
		log.Printf("failed to create pid file: %v", err)
	}

	defer log.Printf("QUIT")
	defer agent.RemovePidFile()

	agent, err := agent.DefaultServer(cc.Client.API(), !cc.IO.IsInteractive())
	if err != nil {
		log.Println(err)
		return errors.New("daemon failed to start")
	}

	log.Printf("OK %d", os.Getpid())

	agent.Serve()

	go func() {
		<-ctx.Done()
		agent.Stop()
	}()

	agent.Wait()

	return nil
}

func runFlyAgentStart(cc *cmdctx.CmdContext) error {
	api := cc.Client.API()
	ctx := context.Background()

	c, err := agent.DefaultClient(ctx)
	if err == nil {
		_ = c.Kill(ctx)
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
