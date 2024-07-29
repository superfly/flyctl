package doctor

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/command/dig"
	"github.com/superfly/flyctl/internal/command/ping"
	"github.com/superfly/flyctl/internal/flyutil"
)

func runPersonalOrgPing(ctx context.Context, orgSlug string) (err error) {
	client := flyutil.ClientFromContext(ctx)

	ac, err := agent.DefaultClient(ctx)
	if err != nil {
		// shouldn't happen, already tested agent
		return fmt.Errorf("wireguard ping gateway: can't establish agent client: %w", err)
	}

	org, err := client.GetOrganizationBySlug(ctx, orgSlug)
	if err != nil {
		// shouldn't happen, already verified auth token
		return fmt.Errorf("wireguard ping gateway: can't get org %s: %w", orgSlug, err)
	}

	pinger, err := ac.Pinger(ctx, orgSlug, "")
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

func runPersonalOrgCheckDns(ctx context.Context, orgSlug string) error {
	client := flyutil.ClientFromContext(ctx)

	ac, err := agent.DefaultClient(ctx)
	if err != nil {
		// shouldn't happen, already tested agent
		return fmt.Errorf("wireguard dialer: can't establish agent client: %w", err)
	}

	org, err := client.GetOrganizationBySlug(ctx, orgSlug)
	if err != nil {
		// shouldn't happen, already verified auth token
		return fmt.Errorf("wireguard dialer: can't get org %s: %w", orgSlug, err)
	}

	_, err = ac.Resolve(ctx, org.Slug, "_api.internal", "")
	if err != nil {
		return fmt.Errorf("wireguard dialer: failed to lookup _api.internal: %w", err)
	}

	return nil
}

func runPersonalOrgCheckFlaps(ctx context.Context, orgSlug string) error {
	apiClient := flyutil.ClientFromContext(ctx)

	// Set up the agent connection
	ac, err := agent.DefaultClient(ctx)
	if err != nil {
		// shouldn't happen, already tested agent
		return fmt.Errorf("wireguard dialer: can't establish agent client: %w", err)
	}

	// Connect to the personal org via WireGuard
	org, err := apiClient.GetOrganizationBySlug(ctx, orgSlug)
	if err != nil {
		// shouldn't happen, already verified auth token
		return fmt.Errorf("wireguard dialer: can't get org %s: %w", orgSlug, err)
	}

	wgDialer, err := ac.ConnectToTunnel(ctx, org.Slug, "", true)
	if err != nil {
		return fmt.Errorf("wireguard dialer: %w", err)
	}

	// Resolve the IP address of _api.internal
	ip, err := ac.Resolve(ctx, org.Slug, "_api.internal:4280", "")
	if err != nil {
		return fmt.Errorf("wireguard dialer: failed to resolve _api.internal: %w", err)
	}

	// Dial the IP address of _api.internal
	conn, err := wgDialer.DialContext(ctx, "tcp", ip)
	if err != nil {
		return fmt.Errorf("wireguard dialer: failed to dial _api.internal: %w", err)
	}

	// Make an HTTP request to _api.internal:4280. This should return a 404.
	req, err := http.NewRequest("GET", "http://_api.internal:4280", http.NoBody) // skipcq: GO-S1028
	if err != nil {
		return fmt.Errorf("wireguard dialer: failed to create HTTP request: %w", err)
	}

	// Send the HTTP request
	if err = req.Write(conn); err != nil {
		return fmt.Errorf("wireguard dialer: failed to write HTTP request: %w", err)
	}

	// Read the HTTP response
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		return fmt.Errorf("wireguard dialer: failed to read HTTP response: %w", err)
	}

	if resp.StatusCode != 404 {
		return fmt.Errorf("wireguard dialer: expected 404, got %d", resp.StatusCode)
	}

	return nil
}
