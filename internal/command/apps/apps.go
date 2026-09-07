// Package apps implements the apps command chain.
package apps

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flyutil"
)

// New initializes and returns a new apps Command.
func New() *cobra.Command {
	const (
		long  = "Manage your Fly applications."
		short = "Manage apps."
	)

	// TODO: list should also accept the --org param
	// TODO: list should also accept the --platform param

	apps := command.New("apps", short, long, nil)
	apps.Aliases = []string{"app"}

	apps.AddCommand(
		newList(),
		newCreate(),
		newDestroy(),
		newRestart(),
		newMove(),
		newResume(),
		newSuspend(),
		NewOpen(),
		NewReleases(),
		newErrors(),
	)

	return apps
}

// BuildContext is a helper that builds out commonly required context requirements
func BuildContext(ctx context.Context, app *fly.AppCompact) (context.Context, error) {
	return BuildContextForNetwork(ctx, app.Organization.Slug, app.Network)
}

// BuildContextForNetwork establishes an agent tunnel into the given org network
// and stores the resulting dialer in the context.
func BuildContextForNetwork(ctx context.Context, orgSlug, network string) (context.Context, error) {
	client := flyutil.ClientFromContext(ctx)

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("can't establish agent %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, orgSlug, network)
	if err != nil {
		return nil, fmt.Errorf("can't build tunnel for %s: %s", orgSlug, err)
	}
	ctx = agent.DialerWithContext(ctx, dialer)

	return ctx, nil
}
