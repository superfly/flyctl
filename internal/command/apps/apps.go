// Package apps implements the apps command chain.
package apps

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flapsutil"
)

// New initializes and returns a new apps Command.
func New() *cobra.Command {
	const (
		long = `The APPS commands focus on managing your Fly applications.
Start with the CREATE command to register your application.
The LIST command will list all currently registered applications.
`
		short = "Manage apps"
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
	client := fly.ClientFromContext(ctx)

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("can't establish agent %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		return nil, fmt.Errorf("can't build tunnel for %s: %s", app.Organization.Slug, err)
	}
	ctx = agent.DialerWithContext(ctx, dialer)

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppCompact: app,
		AppName:    app.Name,
	})
	if err != nil {
		return nil, err
	}

	ctx = flaps.NewContext(ctx, flapsClient)

	return ctx, nil
}
