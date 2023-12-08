package doctor

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command/dig"
	"github.com/superfly/flyctl/internal/command/ping"
)

// TODO: These probably shouldn't be hardcoded to use the "personal" org,
//       as it's completely valid to use flyctl with a token that doesn't have access to a personal org.

func runPersonalOrgPing(ctx context.Context) (err error) {
	client := client.FromContext(ctx).API()

	ac, err := agent.DefaultClient(ctx)
	if err != nil {
		// shouldn't happen, already tested agent
		return fmt.Errorf("wireguard ping gateway: weird error: %w", err)
	}

	org, err := client.GetOrganizationBySlug(ctx, "personal")
	if err != nil {
		// shouldn't happen, already verified auth token
		return fmt.Errorf("wireguard ping gateway: weird error: %w", err)
	}

	pinger, err := ac.Pinger(ctx, "personal")
	if err != nil {
		return fmt.Errorf("wireguard ping gateway: %w", err)
	}

	defer pinger.Close()

	_, ns, err := dig.ResolverForOrg(ctx, ac, org.Slug)
	if err != nil {
		return fmt.Errorf("wireguard ping gateway: %w", err)
	}

	replyBuf := make([]byte, 1000)

	for i := 0; i < 30; i++ {
		_, err = pinger.WriteTo(ping.EchoRequest(0, i, time.Now(), 12), &net.IPAddr{IP: net.ParseIP(ns)})

		pinger.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		_, _, err := pinger.ReadFrom(replyBuf)
		if err != nil {
			continue
		}

		return nil
	}

	return fmt.Errorf("ping gateway: no response from gateway received")
}

func getWireguardDialer(ctx context.Context) (agent.Dialer, error) {
	client := client.FromContext(ctx).API()

	ac, err := agent.DefaultClient(ctx)
	if err != nil {
		// shouldn't happen, already tested agent
		return nil, fmt.Errorf("wireguard dialer: weird error: %w", err)
	}

	org, err := client.GetOrganizationBySlug(ctx, "personal")
	if err != nil {
		// shouldn't happen, already verified auth token
		return nil, fmt.Errorf("wireguard dialer: weird error: %w", err)
	}

	dialer, err := ac.ConnectToTunnel(ctx, org.Slug)
	if err != nil {
		return nil, fmt.Errorf("wireguard dialer: %w", err)
	}

	return dialer, nil
}

func runPersonalOrgCheckDns(ctx context.Context, dialer agent.Dialer) error {

	panic("TODO: DNS check")
}

func runPersonalOrgCheckFlaps(ctx context.Context, dialer agent.Dialer) error {

	panic("TODO: HTTP/Flaps check")
}
