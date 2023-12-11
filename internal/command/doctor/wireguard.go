package doctor

import (
	"context"
	"fmt"
	"net"
	"net/http"
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

func runPersonalOrgCheckDns(ctx context.Context) error {
	client := client.FromContext(ctx).API()

	ac, err := agent.DefaultClient(ctx)
	if err != nil {
		// shouldn't happen, already tested agent
		return fmt.Errorf("wireguard dialer: weird error: %w", err)
	}

	org, err := client.GetOrganizationBySlug(ctx, "personal")
	if err != nil {
		// shouldn't happen, already verified auth token
		return fmt.Errorf("wireguard dialer: weird error: %w", err)
	}

	records, err := ac.LookupTxt(ctx, org.Slug, "_peer.internal")
	if err != nil {
		return fmt.Errorf("wireguard dialer: %w", err)
	}
	if len(records) == 0 {
		return fmt.Errorf("wireguard dialer: no TXT records found for _peer.internal")
	}

	return nil
}

func runPersonalOrgCheckFlaps(ctx context.Context) error {

	apiClient := client.FromContext(ctx).API()

	ac, err := agent.DefaultClient(ctx)
	if err != nil {
		// shouldn't happen, already tested agent
		return fmt.Errorf("wireguard dialer: weird error: %w", err)
	}

	org, err := apiClient.GetOrganizationBySlug(ctx, "personal")
	if err != nil {
		// shouldn't happen, already verified auth token
		return fmt.Errorf("wireguard dialer: weird error: %w", err)
	}

	wgDialer, err := ac.ConnectToTunnel(ctx, org.Slug, true)
	if err != nil {
		return fmt.Errorf("wireguard dialer: %w", err)
	}

	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				return wgDialer.DialContext(ctx, network, address)
			},
		},
	}

	httpClient := http.Client{Transport: &http.Transport{DialContext: dialer.DialContext}}

	// Make an HTTP request to _api.internal:4280.
	// It should 404. This is a success.

	resp, err := httpClient.Get("http://_api.internal:4280")
	if err != nil {
		return fmt.Errorf("wireguard dialer: failed to make HTTP request: %w", err)
	}
	if resp.StatusCode != 404 {
		return fmt.Errorf("wireguard dialer: expected 404, got %d", resp.StatusCode)
	}

	return nil
}
