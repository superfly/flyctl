package cmd

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/agent"
)

func newAgentCommand(client *client.Client) *Command {
	cmd := BuildCommandKS(nil, nil, docstrings.Get("agent"), client, nil, requireSession)
	cmd.Hidden = true

	_ = BuildCommandKS(cmd,
		runFlyAgentDaemonStart,
		docstrings.Get("agent.daemon-start"),
		client,
		nil,
		requireSession)

	_ = BuildCommandKS(cmd,
		runFlyAgentStart,
		docstrings.Get("agent.start"),
		client,
		nil,
		requireSession)

	_ = BuildCommandKS(cmd,
		runFlyAgentStart,
		docstrings.Get("agent.restart"),
		client,
		nil,
		requireSession)

	_ = BuildCommandKS(cmd,
		runFlyAgentStop,
		docstrings.Get("agent.stop"),
		client,
		nil,
		requireSession)

	return cmd
}

func runFlyAgentDaemonStart(ctx *cmdctx.CmdContext) error {
	agent, err := agent.DefaultServer(ctx.Client.API())
	if err != nil {
		log.Fatalf("can't start daemon: %s", err)
	}

	fmt.Printf("OK %d\n", os.Getpid())

	agent.Serve()

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
		fmt.Fprintf(os.Stderr, "can't start agent: %s", err)
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
