package agent

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/iostreams"
)

func BringUpAgent(ctx context.Context, client flyutil.Client, app *fly.AppCompact, quiet bool) (*Client, Dialer, error) {
	io := iostreams.FromContext(ctx)

	agentclient, err := Establish(ctx, client)
	slug := app.Organization.Slug
	name := app.Name
	if err != nil {
		captureError(ctx, err, "agent-remote", slug, name)
		return nil, nil, errors.Wrap(err, "can't establish agent")
	}

	dialer, err := agentclient.Dialer(ctx, slug, app.Network)
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
