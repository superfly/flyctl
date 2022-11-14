package apps

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/machine"
)

func newRestart() *cobra.Command {
	const (
		long  = `The APPS RESTART command will perform a rolling restart against all running VM's`
		short = "Restart an application"
		usage = "restart [APPNAME]"
	)

	cmd := command.New(usage, short, long, RunRestart,
		command.RequireSession,
	)
	cmd.Args = cobra.ExactArgs(1)

	// Note -
	flag.Add(cmd,
		flag.Bool{
			Name:        "force",
			Shorthand:   "f",
			Description: "Will issue a restart against each Machine even if there are errors. ( Machines only )",
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

func RunRestart(ctx context.Context) error {
	var (
		appName = flag.FirstArg(ctx)
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

	input := &api.RestartMachineInput{
		ForceStop:        flag.GetBool(ctx, "force-stop"),
		SkipHealthChecks: flag.GetBool(ctx, "skip-health-checks"),
	}

	if app.PlatformVersion == "machines" {
		// PG specific restart process
		if app.PostgresAppRole != nil && app.PostgresAppRole.Name == "postgres_cluster" {
			return fmt.Errorf("postgres apps should use `fly pg restart` instead")
		}

		// Generic machine restart process
		if err := machine.RollingRestart(ctx, input); err != nil {
			return err
		}
	}

	// Nomad specific restart logic
	return runNomadRestart(ctx, app)
}

func runNomadRestart(ctx context.Context, app *api.AppCompact) error {
	// PG specific restart process
	if app.PostgresAppRole != nil && app.PostgresAppRole.Name == "postgres_cluster" {
		return fmt.Errorf("postgres apps should use `fly pg restart` instead")
	}

	client := client.FromContext(ctx).API()

	if _, err := client.RestartApp(ctx, app.Name); err != nil {
		return fmt.Errorf("failed restarting app: %w", err)
	}

	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.Out, "%s is being restarted\n", app.Name)

	return nil
}

// TODO - Work to move this kind of thing into the apps package.
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
