package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"syscall"
	"time"

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
	c, err := agent.DefaultClient()
	if err == nil {
		c.Kill()
	}

	_, err = EstablishFlyAgent(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "can't start agent: %s", err)
	}

	return err
}

func runFlyAgentStop(ctx *cmdctx.CmdContext) error {
	c, err := agent.DefaultClient()
	if err == nil {
		c.Kill()
	}

	return err
}

func EstablishFlyAgent(ctx *cmdctx.CmdContext) (*agent.Client, error) {
	c, err := agent.DefaultClient()
	if err == nil {
		_, err := c.Ping()
		if err == nil {
			return c, nil
		}
	}

	cmd := exec.Command(os.Args[0], "agent", "daemon-start")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	if err = cmd.Start(); err != nil {
		return nil, err
	}

	// this is gross placeholder logic

	for i := 0; i < 5; i++ {
		time.Sleep(100 * time.Millisecond)

		c, err := agent.DefaultClient()
		if err == nil {
			_, err := c.Ping()
			if err == nil {
				return c, nil
			}
		}
	}

	return nil, fmt.Errorf("couldn't establish connection to Fly Agent")
}
