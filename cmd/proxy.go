package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/terminal"
)

func newProxyCommand(client *client.Client) *Command {

	proxyDocStrings := docstrings.Get("proxy")
	cmd := BuildCommandKS(nil, nil, proxyDocStrings, client, requireSession, requireAppName)

	proxyStartStrings := docstrings.Get("proxy.start")
	start := BuildCommandKS(cmd, runProxy, proxyStartStrings, client, requireSession, requireAppName)
	start.Args = cobra.ExactArgs(1)

	proxyStoptStrings := docstrings.Get("proxy.stop")
	stop := BuildCommandKS(cmd, runUnproxy, proxyStoptStrings, client, requireSession)
	stop.Args = cobra.ExactArgs(0)

	return cmd
}

func runProxy(cmdCtx *cmdctx.CmdContext) error {
	ctx := createCancellableContext()

	client := cmdCtx.Client.API()

	terminal.Debugf("Retrieving app info for %s\n", cmdCtx.AppName)

	app, err := client.GetApp(cmdCtx.AppName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	c, err := agent.Establish(ctx, client)
	if err != nil {
		return err
	}

	err = c.Establish(ctx, app.Organization.Slug)
	if err != nil {
		return err
	}

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = "Launching proxy..."
	s.Start()

	err = c.Proxy(ctx, cmdCtx.Args[0], app.Name)
	if err != nil {
		return err
	}

	s.FinalMSG = fmt.Sprintf("Started proxy to %s on port %s \n", app.Name, cmdCtx.Args[0])
	s.Stop()

	return nil

}

func runUnproxy(cmdCtx *cmdctx.CmdContext) error {
	ctx := createCancellableContext()

	client := cmdCtx.Client.API()

	c, err := agent.Establish(ctx, client)
	if err != nil {
		return err
	}

	err = c.Unproxy(ctx)
	if err != nil {
		return err
	}

	fmt.Println("Proxy succesfully stopped")

	return nil
}
