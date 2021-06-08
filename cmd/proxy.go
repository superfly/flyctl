package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/proxy"
	"github.com/superfly/flyctl/terminal"
)

func newProxyCommand(client *client.Client) *Command {

	proxyDocStrings := docstrings.Get("proxy")
	cmd := BuildCommandKS(nil, runProxy, proxyDocStrings, client, requireSession, requireAppName)

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func runProxy(ctx *cmdctx.CmdContext) error {
	ports := strings.Split(ctx.Args[0], ":")

	lPort, rPort := ports[0], ports[1]

	if len(rPort) == 0 {
		rPort = lPort
	}

	client := ctx.Client.API()

	terminal.Debugf("Retrieving app info for %s\n", ctx.AppName)

	app, err := client.GetApp(ctx.AppName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	agent, err := EstablishFlyAgent(ctx)
	if err != nil {
		return err
	}

	dialer, err := agent.Dialer(&app.Organization)
	if err != nil {
		return err
	}

	rAddr := fmt.Sprintf("%s.internal", ctx.AppName)

	fmt.Printf("Proxying local connections '%s:%s' to %s\n", lPort, rPort, ctx.AppName)

	proxy := &proxy.Server{
		LocalAddr:  formatAddr("127.0.0.1", lPort),
		RemoteAddr: formatAddr(rAddr, rPort),
		Dial:       dialer.DialContext,
	}

	return proxy.ListenAndServe(context.Background())
}

func formatAddr(host, port string) string {
	return fmt.Sprintf("%s:%s", host, port)
}
