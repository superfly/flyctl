package agent

import (
	"context"
	"fmt"
	"log"

	"github.com/pkg/errors"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/iostreams"
)

func BringUpAgent(ctx context.Context, client flyutil.Client, app *flaps.App, quiet bool) (*Client, Dialer, error) {
	io := iostreams.FromContext(ctx)

	name := app.Name

	// check if this is a personal org
	// todo(lillian):
	org, err := client.GetOrganizationByApp(ctx, name)
	if err != nil {
		return nil, nil, fmt.Errorf("get organization: %w", err)
	}

	slug := org.Slug

	agentclient, err := Establish(ctx, client)

	if err != nil {
		captureError(ctx, err, "agent-remote", slug, name)
		return nil, nil, errors.Wrap(err, "can't establish agent")
	}

	log.Printf("establishing agent, %s:%s", slug, name)

	dialer, err := agentclient.Dialer(ctx, org.Slug, app.Network)
	if err != nil {
		captureError(ctx, err, "agent-remote", slug, name)
		return nil, nil, fmt.Errorf("ssh: can't build tunnel for %s: %s\n", slug, err)
	}

	if !quiet {
		io.StartProgressIndicatorMsg("Connecting to tunnel")
	}
	if err := agentclient.WaitForTunnel(ctx, slug, app.Network); err != nil {
		captureError(ctx, err, "agent-remote", slug, name)
		return nil, nil, errors.Wrapf(err, "tunnel unavailable")
	}
	if !quiet {
		io.StopProgressIndicator()
	}

	return agentclient, dialer, nil
}
