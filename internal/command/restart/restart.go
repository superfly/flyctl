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
	"github.com/superfly/flyctl/internal/command/postgres"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/machine"
)

func New() *cobra.Command {
	const (
		long  = `The APPS RESTART command will perform a rolling restart against all running VM's`
		short = "Restart an application"
		usage = "restart [APPNAME]"
	)

	cmd := command.New(usage, short, long, Run,
		command.RequireSession,
	)
	cmd.Args = cobra.RangeArgs(0, 1)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.Bool{
			Name:        "force",
			Shorthand:   "f",
			Description: "Will  ",
			Default:     false,
		},
		flag.Bool{
			Name:        "force-stop",
			Description: "Performs a force stop against the target Machine. ( Machines only )",
			Default:     false,
		},
		flag.Bool{
			Name:        "skip-health-checks",
			Description: "Restarts app without waiting for health checks. ( Machines only )",
			Default:     false,
		},
	)

	return cmd
}

func Run(ctx context.Context) error {
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
		if app.PostgresAppRole != nil && app.PostgresAppRole.Name == "postgres_cluster" {
			return postgres.MachinesRestart(ctx)
		}

		if err := machine.RollingRestart(ctx); err != nil {
			return err
		}
	}

	return runNomadRestart(ctx, app)
}

func runNomadRestart(ctx context.Context, app *api.AppCompact) error {
	if app.PostgresAppRole != nil && app.PostgresAppRole.Name == "postgres_cluster" {
		return postgres.NomadRestart(ctx, app)
	}

	client := client.FromContext(ctx).API()

	if _, err := client.RestartApp(ctx, app.Name); err != nil {
		return fmt.Errorf("failed restarting app: %w", err)
	}

	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.Out, "%s is being restarted\n", app.Name)

	return nil
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
