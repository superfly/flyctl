package cmd

import (
	"context"
	"fmt"
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

	return cmd
}

func runFlyAgentDaemonStart(ctx *cmdctx.CmdContext) error {
	agent, err := agent.DefaultServer(ctx.Client.API())
	if err != nil {
		return errors.Wrap(err, "daemon error")
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
