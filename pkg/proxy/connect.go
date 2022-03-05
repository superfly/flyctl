package proxy

import (
	"context"
	"fmt"
	"net"

	"github.com/AlecAivazis/survey/v2"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/iostreams"
	"github.com/superfly/flyctl/pkg/ip"
)

type ConnectParams struct {
	App            *api.App
	Dialer         agent.Dialer
	Ports          []string
	RemoteHost     string
	PromptInstance bool
	DisableSpinner bool
}

func Connect(ctx context.Context, p *ConnectParams) (err error) {

	var (
		io     = iostreams.FromContext(ctx)
		client = client.FromContext(ctx).API()
	)
	var localPort, remotePort, remoteAddr string

	localPort = p.Ports[0]

	if len(p.Ports) > 1 {
		remotePort = p.Ports[1]
	} else {
		remotePort = localPort
	}

	agentclient, err := agent.Establish(ctx, client)

	if err != nil {
		return err
	}

	// Prompt for a specific instance and set it as the remote target
	if p.PromptInstance {
		instance, err := selectInstance(ctx, p.App, agentclient)

		if err != nil {
			return err
		}

		remoteAddr = fmt.Sprintf("[%s]:%s", instance, remotePort)
	}

	if remoteAddr == "" && p.RemoteHost != "" {

		// If a host is specified that isn't an IpV6 address, assume it's a DNS entry and wait for that
		// entry to resolve
		if !ip.IsV6(p.RemoteHost) {
			if err := agentclient.WaitForDNS(ctx, p.Dialer, p.App.Organization.Slug, p.RemoteHost); err != nil {
				return fmt.Errorf("%s: %w", p.RemoteHost, err)
			}
		}

		remoteAddr = fmt.Sprintf("[%s]:%s", p.RemoteHost, remotePort)
	}

	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("127.0.0.1:%s", localPort))
	if err != nil {
		return err
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Proxying local port %s to remote %s\n", localPort, remoteAddr)

	proxy := Server{
		Addr:     remoteAddr,
		Listener: listener,
		Dial:     p.Dialer.DialContext,
	}

	return proxy.ProxyServer(ctx)
}

func selectInstance(ctx context.Context, app *api.App, c *agent.Client) (instance string, err error) {
	instances, err := c.Instances(ctx, &app.Organization, app.Name)
	if err != nil {
		return "", fmt.Errorf("look up %s: %w", app.Name, err)
	}

	selected := 0
	prompt := &survey.Select{
		Message:  "Select instance:",
		Options:  instances.Labels,
		PageSize: 15,
	}

	if err := survey.AskOne(prompt, &selected); err != nil {
		return "", fmt.Errorf("selecting instance: %w", err)
	}

	return instances.Addresses[selected], nil
}
