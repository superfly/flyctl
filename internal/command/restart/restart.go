package restart

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func New() *cobra.Command {
	const (
		short = "Restarts each VM one by one."
		long  = short + " Downtime should be minimal." + "\n"
		usage = "restart"
	)

	cmd := command.New(usage, short, long, runRestart,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Bool{
			Name:        "force",
			Shorthand:   "f",
			Description: "Force a restart even we don't have an active leader",
			Default:     false,
		},
	)

	return cmd
}

func runRestart(ctx context.Context) error {
	var (
		appName = app.NameFromContext(ctx)
		client  = client.FromContext(ctx).API()
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	ctx, err = buildContext(ctx, app)
	if err != nil {
		return err
	}

	if app.PlatformVersion == "machines" {
		return runMachineRestart(ctx, app)
	}

	return runNomadRestart(ctx, app)
}

func buildContext(ctx context.Context, app *api.AppCompact) (context.Context, error) {
	client := client.FromContext(ctx).API()

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("can't establish agent %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		return nil, fmt.Errorf("can't build tunnel for %s: %s", app.Organization.Slug, err)
	}
	ctx = agent.DialerWithContext(ctx, dialer)

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return nil, err
	}

	ctx = flaps.NewContext(ctx, flapsClient)

	return ctx, nil
}
