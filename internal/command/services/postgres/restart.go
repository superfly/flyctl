package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/flypg"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newRestart() (cmd *cobra.Command) {
	const (
		long = `Restarts each member of the Postgres cluster one by one. Downtime should be minimal.
`
		short = "Restarts the Postgres cluster"
		usage = "restart"
	)

	cmd = command.New(usage, short, long, runRestart,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return
}

func runRestart(ctx context.Context) error {
	appName := app.NameFromContext(ctx)
	client := client.FromContext(ctx).API()
	io := iostreams.FromContext(ctx)

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	machines, err := client.ListMachines(ctx, app.ID, "started")
	if err != nil {
		return err
	}

	if len(machines) == 0 {
		return fmt.Errorf("no machines found")
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return fmt.Errorf("can't establish agent %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		return fmt.Errorf("ssh: can't build tunnel for %s: %s", app.Organization.Slug, err)
	}

	for _, machine := range machines {
		fmt.Fprintf(io.Out, "Restarting machine %q... ", machine.ID)

		address := formatAddress(machine)

		pgclient := flypg.NewFromInstance(address, dialer)

		if err := pgclient.RestartNode(ctx); err != nil {
			fmt.Fprintln(io.Out, "failed")
			return err
		}
		fmt.Fprintln(io.Out, "complete")
	}

	return nil
}
