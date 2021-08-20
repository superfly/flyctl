package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/terminal"
)

func newProxyCommand(client *client.Client) *Command {

	proxyDocStrings := docstrings.Get("proxy")
	cmd := BuildCommandKS(nil, runProxy, proxyDocStrings, client, requireSession, requireAppName)

	cmd.Args = cobra.ExactArgs(1)

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

	err = c.Proxy(ctx, cmdCtx.Args[0], app.Name)
	if err != nil {
		return err
	}

	return nil

}
