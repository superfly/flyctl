package cmd

import (
	"fmt"
	"log"
	"os"

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
	agent, err := agent.DefaultServer(ctx)
	if err != nil {
		log.Fatalf("can't start daemon: %s", err)
	}

	fmt.Printf("OK %d\n", os.Getpid())

	agent.Serve()

	return nil
}

func runFlyAgentStart(ctx *cmdctx.CmdContext) error {
	api := ctx.Client.API()

	c, err := agent.DefaultClient(api)
	if err == nil {
		c.Kill()
	}

	_, err = agent.Establish(api)
	if err != nil {
		fmt.Fprintf(os.Stderr, "can't start agent: %s", err)
	}

	return err
}

func runFlyAgentStop(ctx *cmdctx.CmdContext) error {
	api := ctx.Client.API()

	c, err := agent.DefaultClient(api)
	if err == nil {
		c.Kill()
	}

	return err
}
