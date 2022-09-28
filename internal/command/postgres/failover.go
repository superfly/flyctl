package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newFailover() *cobra.Command {
	const (
		short = "Failover to a new primary"
		long  = short + "\n"
		usage = "failover"
	)

	var cmd = command.New(usage, short, long, runFailover,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runFailover(ctx context.Context) (err error) {
	var (
		MinPostgresHaVersion = "0.0.20"
		io                   = iostreams.FromContext(ctx)
		client               = client.FromContext(ctx).API()
		appName              = app.NameFromContext(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	if !app.IsPostgresApp() {
		return fmt.Errorf("app %s is not a Postgres app", app.Name)
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return fmt.Errorf("error establishing agent: %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		return fmt.Errorf("flaps: can't build tunnel for %s: %w", app.Organization.Slug, err)
	}
	ctx = agent.DialerWithContext(ctx, dialer)

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("list of machines could not be retrieved: %w", err)
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	machines, err := flapsClient.List(ctx, "started")
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}

	leader, err := fetchPGLeader(ctx, machines)
	if err != nil {
		return fmt.Errorf("leader could not be retrieved %w", err)
	}

	if err := hasRequiredVersionOnMachines(leader, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
		return err
	}

	// You can not failerover for single node postgres
	if len(machines) <= 1 {
		return fmt.Errorf("failover is not available for standalone postgres")
	}

	pgclient := flypg.New(appName, dialer)

	fmt.Fprintf(io.Out, "Performing a failover\n")
	if err := pgclient.Failover(ctx); err != nil {
		return fmt.Errorf("failed to trigger failover %w", err)
	}

	fmt.Fprintf(io.Out, "Failover complete\n")
	return
}
