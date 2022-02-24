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
	machines "github.com/superfly/flyctl/pkg/machine"
	"github.com/superfly/flyctl/terminal"
)

func Connect(ctx context.Context, ports []string, app *api.App, dialer agent.Dialer, selectInstanceRequested bool, machine *api.Machine) (err error) {

	var (
		io     = iostreams.FromContext(ctx)
		client = client.FromContext(ctx).API()
	)

	// Remote may be an IPv6 address or a port
	var localPort, remoteString, remoteAddr string

	if len(ports) < 2 {
		localPort, remoteString = ports[0], ports[0]
	} else {
		localPort, remoteString = ports[0], ports[1]
	}

	agentclient, err := agent.Establish(ctx, client)

	if err != nil {
		return err
	}

	// If instance selection was required, assume the remote is a port and append the port
	// to the select instance IP
	if selectInstanceRequested {
		instance, err := selectInstance(ctx, app, agentclient)

		if err != nil {
			return err
		}

		remoteAddr = fmt.Sprintf("[%s]:%s", instance, remoteString)

	} else {

		// If the remote is specified as an IPV6 address, use it and the local port on the remote
		if agent.IsIPv6(remoteString) {
			remoteAddr = fmt.Sprintf("[%s]:%s", remoteString, localPort)
		} else {
			// Otherwise find the correct IP or hostname for the remote based on the application,
			// and assume the remote is a port
			if machine != nil {
				remoteAddr = fmt.Sprintf("[%s]:%s", machines.IpAddress(machine), remoteString)
			} else {
				remoteAddr = fmt.Sprintf("top1.nearest.of.%s.internal:%s", app.Name, remoteString)
				io.StartProgressIndicatorMsg(fmt.Sprintf("Waiting for host %s", remoteAddr))
				if err := agentclient.WaitForDNS(ctx, app.Organization.Slug, remoteAddr); err != nil {
					return fmt.Errorf("host unavailable %w", err)
				}
				io.StopProgressIndicator()
			}
		}
	}

	if err := proxyConnect(ctx, localPort, remoteAddr, dialer); err != nil {
		return err
	}

	return
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

func proxyConnect(ctx context.Context, localPort string, remoteAddr string, dialer agent.Dialer) error {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("127.0.0.1:%s", localPort))
	if err != nil {
		return err
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return err
	}

	fmt.Printf("Proxy listening on: %s\n", listener.Addr().String())

	proxy := Server{
		Addr:     remoteAddr,
		Listener: listener,
		Dial:     dialer.DialContext,
	}

	terminal.Debug("Starting proxy on: ", localPort)
	terminal.Debug("Connecting to ", remoteAddr)

	return proxy.ProxyServer(ctx)
}
