package cmd

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/sentry"
	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/proxy"
	"github.com/superfly/flyctl/terminal"
)

func newProxyCommand(client *client.Client) *Command {

	proxyDocStrings := docstrings.Get("proxy")
	cmd := BuildCommandKS(nil, runProxy, proxyDocStrings, client, requireSession, requireAppName)
	cmd.Args = cobra.ExactArgs(1)

	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "select",
		Shorthand:   "s",
		Default:     false,
		Description: "select available instances",
	})

	return cmd
}

func runProxy(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	ports := strings.Split(cmdCtx.Args[0], ":")

	var local, remote string

	if len(ports) < 2 {
		local, remote = ports[0], ports[0]
	} else {
		local, remote = ports[0], ports[1]
	}

	client := cmdCtx.Client.API()

	terminal.Debugf("Retrieving app info for %s\n", cmdCtx.AppName)

	app, err := client.GetApp(ctx, cmdCtx.AppName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}
	captureError := func(err error) {
		// ignore cancelled errors
		if errors.Is(err, context.Canceled) {
			return
		}

		sentry.CaptureException(err,
			sentry.WithTag("feature", "ssh-console"),
			sentry.WithContexts(map[string]interface{}{
				"app": map[string]interface{}{
					"name": app.Name,
				},
				"organization": map[string]interface{}{
					"name": app.Organization.Slug,
				},
				"port": map[string]interface{}{
					"local":  local,
					"remote": remote,
				},
			}),
		)
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		captureError(err)
		return err
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		captureError(err)
		return err
	}

	cmdCtx.IO.StartProgressIndicatorMsg("Connecting to tunnel")
	if err := agentclient.WaitForTunnel(ctx, &app.Organization); err != nil {
		captureError(err)
		return fmt.Errorf("tunnel unavailable %w", err)
	}
	cmdCtx.IO.StopProgressIndicator()

	if cmdCtx.Config.GetBool("select") {
		instances, err := agentclient.Instances(ctx, &app.Organization, cmdCtx.AppName)
		if err != nil {
			captureError(err)
			return fmt.Errorf("look up %s: %w", cmdCtx.AppName, err)
		}

		selected := 0
		prompt := &survey.Select{
			Message:  "Select instance:",
			Options:  instances.Labels,
			PageSize: 15,
		}

		if err := survey.AskOne(prompt, &selected); err != nil {
			return fmt.Errorf("selecting instance: %w", err)
		}

		if err := survey.AskOne(prompt, &selected); err != nil {
			return fmt.Errorf("selecting instance: %w", err)
		}

		remote = fmt.Sprintf("[%s]:%s", instances.Addresses[selected], remote)
	} else {
		remote = fmt.Sprintf("top1.nearest.of.%s.internal:%s", cmdCtx.AppName, remote)

	}

	if !agent.IsIPv6(remote) {
		cmdCtx.IO.StartProgressIndicatorMsg("Waiting for host")
		if err := agentclient.WaitForHost(ctx, &app.Organization, remote); err != nil {
			captureError(err)
			return fmt.Errorf("host unavailable %w", err)
		}
		cmdCtx.IO.StopProgressIndicator()
	}

	params := &ProxyParams{
		LocalAddr:  local,
		RemoteAddr: remote,
		Dialer:     dialer,
	}

	if err := proxyConnect(ctx, params); err != nil {
		captureError(err)
		return err
	}

	return nil
}

type ProxyParams struct {
	RemoteAddr string
	LocalAddr  string
	Dialer     agent.Dialer
}

func proxyConnect(ctx context.Context, params *ProxyParams) error {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("127.0.0.1:%s", params.LocalAddr))
	if err != nil {
		return err
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return err
	}

	fmt.Printf("Proxy listening on: %s\n", listener.Addr().String())

	proxy := proxy.Server{
		Addr:     params.RemoteAddr,
		Listener: listener,
		Dial:     params.Dialer.DialContext,
	}

	terminal.Debug("Starting proxy on: ", params.LocalAddr)

	terminal.Debug("Connecting to ", params.RemoteAddr)

	return proxy.Proxy(ctx)
}
