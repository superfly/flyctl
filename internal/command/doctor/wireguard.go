package doctor

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/command/dig"
	"github.com/superfly/flyctl/internal/command/ping"
	"github.com/superfly/flyctl/internal/flag"
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

	// Set up the agent connection
	ac, err := agent.DefaultClient(ctx)
	if err != nil {
		// shouldn't happen, already tested agent
		return fmt.Errorf("wireguard dialer: weird error: %w", err)
	}

	// Connect to the personal org via WireGuard
	org, err := apiClient.GetOrganizationBySlug(ctx, "personal")
	if err != nil {
		// shouldn't happen, already verified auth token
		return fmt.Errorf("wireguard dialer: weird error: %w", err)
	}

	wgDialer, err := ac.ConnectToTunnel(ctx, org.Slug, true)
	if err != nil {
		return fmt.Errorf("wireguard dialer: %w", err)
	}

	// HACK: Set the quiet flag to true so that the agent doesn't print out progress indicators
	quietCtx := flag.NewContext(ctx, helpers.Clone(flag.FromContext(ctx)))
	_ = flag.FromContext(quietCtx).BoolP("quiet", "q", true, "suppress output")

	// Wait for the connection to be established
	err = ac.WaitForDNS(quietCtx, wgDialer, org.Slug, "_api.internal")
	if err != nil {
		return fmt.Errorf("wireguard dialer: failed to wait for DNS to resolve: %w", err)
	}

	// Resolve the IP address of _api.internal
	ip, err := ac.Resolve(ctx, org.Slug, "_api.internal:4280")
	if err != nil {
		return fmt.Errorf("wireguard dialer: failed to resolve _api.internal: %w", err)
	}

	// Dial the IP address of _api.internal
	conn, err := wgDialer.DialContext(ctx, "tcp", ip)
	if err != nil {
		return fmt.Errorf("wireguard dialer: failed to dial _api.internal: %w", err)
	}

	// Make an HTTP request to _api.internal:4280. This should return a 404.
	req, err := http.NewRequest("GET", "http://_api.internal:4280", nil)
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

	if err != nil {
		return fmt.Errorf("wireguard dialer: failed to make HTTP request: %w", err)
	}
	if resp.StatusCode != 404 {
		return fmt.Errorf("wireguard dialer: expected 404, got %d", resp.StatusCode)
	}

	return nil
}
