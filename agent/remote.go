package agent

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/internal/wireguard"
	"github.com/superfly/flyctl/iostreams"
)

func BringUpAgent(ctx context.Context, client wireguard.WebClient, app *fly.AppCompact, network string, quiet bool) (*Client, Dialer, error) {
	io := iostreams.FromContext(ctx)

	agentclient, err := Establish(ctx, client)
	slug := app.Organization.Slug
	name := app.Name
	if err != nil {
		captureError(ctx, err, "agent-remote", slug, name)
		return nil, nil, errors.Wrap(err, "can't establish agent")
	}

	dialer, err := agentclient.Dialer(ctx, slug, network)
	if err != nil {
		captureError(ctx, err, "agent-remote", slug, name)
		return nil, nil, fmt.Errorf("ssh: can't build tunnel for %s: %s\n", slug, err)
	}

	if !quiet {
		io.StartProgressIndicatorMsg("Connecting to tunnel")
	}
	if err := agentclient.WaitForTunnel(ctx, slug, network); err != nil {
		captureError(ctx, err, "agent-remote", slug, name)
		return nil, nil, errors.Wrapf(err, "tunnel unavailable")
	}
	if !quiet {
		io.StopProgressIndicator()
	}

	return agentclient, dialer, nil
}

func BringUpAgentOrgSlug(ctx context.Context, client wireguard.WebClient, orgSlug string, network string, quiet bool) (*Client, Dialer, error) {
	if orgSlug != "personal" {
		// The agent keys tunnels based on web's org Slug, rather than RawSlug,
		// so we need to use "personal" if the orgSlug belongs to the current
		// user's personal org.
		//
		// It'd be good to standardize on RawSlug instead, but there are lots of
		// callers to the agent and we don't want to touch them all at once.
		uiexClient := uiexutil.ClientFromContext(ctx)
		uiexOrg, err := uiexClient.GetOrganization(ctx, orgSlug)
		if err != nil {
			return nil, nil, err
		}

		orgSlug = uiexOrg.Slug
	}

	appCompact := &fly.AppCompact{
		// Name is only used for additional context in error reporting, so for
		// simplicity we're going to ignore it.
		Name: "",
		Organization: &fly.OrganizationBasic{
			Slug: orgSlug,
		},
	}

	// The default network is referred "default" by flaps and "" by web.
	if network == "default" {
		network = ""
	}

	return BringUpAgent(ctx, client, appCompact, network, quiet)
}
