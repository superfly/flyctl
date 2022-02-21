package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/AlecAivazis/survey/v2"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/sentry"
	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/iostreams"
	"github.com/superfly/flyctl/terminal"
)

func Connect(ctx context.Context, ports []string, app *api.App, selectInstance bool) (err error) {

	var (
		io     = iostreams.FromContext(ctx)
		client = client.FromContext(ctx).API()
	)

	// May contain an IPv6 address, a port, or both
	var local, remote string

	if len(ports) < 2 {
		local, remote = ports[0], ports[0]
	} else {
		local, remote = ports[0], ports[1]
	}

	captureError := func(err error) {
		// ignore cancelled errors
		if errors.Is(err, context.Canceled) {
			return
		}

		sentry.CaptureException(err,
			sentry.WithTag("feature", "proxy"),
			sentry.WithContexts(map[string]interface{}{
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

	io.StartProgressIndicatorMsg("Connecting to tunnel")
	if err := agentclient.WaitForTunnel(ctx, app.Organization.Slug); err != nil {
		captureError(err)
		return fmt.Errorf("tunnel unavailable %w", err)
	}
	io.StopProgressIndicator()

	if selectInstance {
		instances, err := agentclient.Instances(ctx, &app.Organization, app.Name)
		if err != nil {
			captureError(err)
			return fmt.Errorf("look up %s: %w", app.Name, err)
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
		remote = fmt.Sprintf("top1.nearest.of.%s.internal:%s", app.Name, remote)
	}

	if !agent.IsIPv6(remote) {
		io.StartProgressIndicatorMsg("Waiting for host")
		if err := agentclient.WaitForHost(ctx, app.Organization.Slug, remote); err != nil {
			captureError(err)
			return fmt.Errorf("host unavailable %w", err)
		}
		io.StopProgressIndicator()
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

	return
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

	proxy := Server{
		Addr:     params.RemoteAddr,
		Listener: listener,
		Dial:     params.Dialer.DialContext,
	}

	terminal.Debug("Starting proxy on: ", params.LocalAddr)
	terminal.Debug("Connecting to ", params.RemoteAddr)

	return proxy.ProxyServer(ctx)
}
