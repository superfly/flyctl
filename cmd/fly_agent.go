package cmd

import (
	"context"
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
	ctx := createCancellableContext()

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

	c, err := agent.DefaultClient(api)
	if err == nil {
		c.Kill(ctx)
	}

	_, err = agent.Establish(ctx, api)
	if err != nil {
		return errors.Wrap(err, "failed to start agent")
	}

	return err
}

func runFlyAgentStop(cc *cmdctx.CmdContext) error {
	api := cc.Client.API()
	ctx := context.Background()

	c, err := agent.DefaultClient(api)
	if err == nil {
		c.Kill(ctx)
	}

	return err
}

func runFlyAgentPing(cc *cmdctx.CmdContext) error {
	api := cc.Client.API()
	ctx := context.Background()

	c, err := agent.DefaultClient(api)
	if err != nil {
		return err
	}
	resp, err := c.Ping(ctx)
	if err != nil {
		return err
	}
	cc.WriteJSON(resp)

	return nil
}
