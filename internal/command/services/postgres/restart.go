package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
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

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("list of machines could not be retrieved: %w", err)
	}

	// map of machine lease to machine
	var machines = make(map[string]*api.Machine)

	out, err := flapsClient.List(ctx, "started")
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}

	if len(out) == 0 {
		return fmt.Errorf("no machines found")
	}

	fmt.Fprintf(io.Out, "Acquiring lease on postgres cluster\n")

	for _, machine := range out {
		lease, err := flapsClient.GetLease(ctx, machine.ID, api.IntPointer(40))

		if err != nil {
			return fmt.Errorf("failed to obtain lease: %w", err)
		}
		machines[lease.Data.Nonce] = machine
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return fmt.Errorf("can't establish agent %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		return fmt.Errorf("ssh: can't build tunnel for %s: %s", app.Organization.Slug, err)
	}

	fmt.Fprintf(io.Out, "Restarting Postgres\n")

	for lease, machine := range machines {
		fmt.Fprintf(io.Out, " Restarting %s with lease %s\n", machine.ID, lease)

		address := formatAddress(machine)
		pgclient := flypg.NewFromInstance(address, dialer)

		if err := pgclient.RestartNodePG(ctx); err != nil {
			fmt.Fprintf(io.Out, "postgres on node: %s failed\n", machine.ID)
			return err
		}

		if err := flapsClient.ReleaseLease(ctx, machine.ID, lease); err != nil {
			return fmt.Errorf("failed to release lease: %w", err)
		}
	}
	fmt.Fprintf(io.Out, "Restart complete\n")

	return nil
}
