package proxy

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/ip"
)

type ConnectParams struct {
	AppName          string
	OrganizationSlug string
	Dialer           agent.Dialer
	BindAddr         string
	Ports            []string
	RemoteHost       string
	PromptInstance   bool
	DisableSpinner   bool
	Network          string
}

// Binds to a local port and runs a proxy to a remote address over Wireguard.
// Blocks until context is cancelled.
func Connect(ctx context.Context, p *ConnectParams) (err error) {
	server, err := NewServer(ctx, p)
	if err != nil {
		return err
	}

	return server.ProxyServer(ctx)
}

// Binds to a local port and then starts a goroutine to run a proxy to a remote
// address over Wireguard. Proxy runs until context is cancelled.
// Blocks only until local listener is bound and ready to accept connections.
func Start(ctx context.Context, p *ConnectParams) error {
	server, err := NewServer(ctx, p)
	if err != nil {
		return err
	}

	// currently ignores any error returned by ProxyServer
	// TODO return a channel to caller for async error notification
	go server.ProxyServer(ctx)

	return nil
}

func NewServer(ctx context.Context, p *ConnectParams) (*Server, error) {
	var (
		io            = iostreams.FromContext(ctx)
		client        = flyutil.ClientFromContext(ctx)
		orgSlug       = p.OrganizationSlug
		localBindAddr = p.BindAddr
		localPort     = p.Ports[0]
		remotePort    = localPort
		remoteAddr    string
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
			if err := agentclient.WaitForDNS(ctx, p.Dialer, orgSlug, p.RemoteHost, p.Network); err != nil {
				return nil, fmt.Errorf("%s: %w", p.RemoteHost, err)
			}
		}

		remoteAddr = fmt.Sprintf("[%s]:%s", p.RemoteHost, remotePort)
	}

	var listener net.Listener

	if _, err := strconv.Atoi(localPort); err == nil {
		// just numbers
		addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%s", localBindAddr, localPort))
		if err != nil {
			return nil, err
		}

		listener, err = net.ListenTCP("tcp", addr)
		if err != nil {
			return nil, err
		}

		if localPort == "0" {
			localPort = strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
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
	if err := prompt.Select(ctx, &selected, "Select instance:", "", instances.Labels...); err != nil {
		return "", fmt.Errorf("selecting instance: %w", err)
	}

	return instances.Addresses[selected], nil
}
