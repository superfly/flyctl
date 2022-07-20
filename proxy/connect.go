package proxy

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/AlecAivazis/survey/v2"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/ip"
)

type ConnectParams struct {
	AppName          string
	OrganizationSlug string
	Dialer           agent.Dialer
	Ports            []string
	RemoteHost       string
	PromptInstance   bool
	DisableSpinner   bool
}

func Connect(ctx context.Context, p *ConnectParams) (err error) {
	server, err := NewServer(ctx, p)
	if err != nil {
		return err
	}

	return server.ProxyServer(ctx)
}

func NewServer(ctx context.Context, p *ConnectParams) (*Server, error) {
	var (
		io         = iostreams.FromContext(ctx)
		client     = client.FromContext(ctx).API()
		orgSlug    = p.OrganizationSlug
		localPort  = p.Ports[0]
		remotePort = localPort
		remoteAddr string
	)

	if len(p.Ports) > 1 {
		remotePort = p.Ports[1]
	}

	agentclient, err := agent.Establish(ctx, client)

	if err != nil {
		return nil, err
	}

	// Prompt for a specific instance and set it as the remote target
	if p.PromptInstance {
		instance, err := selectInstance(ctx, p.OrganizationSlug, p.AppName, agentclient)

		if err != nil {
			return nil, err
		}

		remoteAddr = fmt.Sprintf("[%s]:%s", instance, remotePort)
	}

	if remoteAddr == "" && p.RemoteHost != "" {

		// If a host is specified that isn't an IpV6 address, assume it's a DNS entry and wait for that
		// entry to resolve
		if !ip.IsV6(p.RemoteHost) {
			if err := agentclient.WaitForDNS(ctx, p.Dialer, orgSlug, p.RemoteHost); err != nil {
				return nil, fmt.Errorf("%s: %w", p.RemoteHost, err)
			}
		}

		remoteAddr = fmt.Sprintf("[%s]:%s", p.RemoteHost, remotePort)
	}

	var listener net.Listener

	if _, err := strconv.Atoi(localPort); err == nil {
		// just numbers
		addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("127.0.0.1:%s", localPort))
		if err != nil {
			return nil, err
		}

		listener, err = net.ListenTCP("tcp", addr)
		if err != nil {
			return nil, err
		}
	} else {
		// probably a unix path
		addr, err := net.ResolveUnixAddr("unix", localPort)
		if err != nil {
			return nil, err
		}

		listener, err = net.ListenUnix("unix", addr)
		if err != nil {
			return nil, err
		}
	}

	fmt.Fprintf(io.Out, "Proxying local port %s to remote %s\n", localPort, remoteAddr)

	return &Server{
		Addr:     remoteAddr,
		Listener: listener,
		Dial:     p.Dialer.DialContext,
	}, nil
}

func selectInstance(ctx context.Context, org, app string, c *agent.Client) (instance string, err error) {
	instances, err := c.Instances(ctx, org, app)
	if err != nil {
		return "", fmt.Errorf("look up %s: %w", app, err)
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
