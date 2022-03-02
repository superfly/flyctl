package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/command/machines"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/client"
)

func newList() (cmd *cobra.Command) {
	const (
		long = `Connect to the Postgres console
`
		short = "Connect to the Postgres console"
		usage = "list"
	)

	cmd = command.New(usage, short, long, runList,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return
}

func runList(ctx context.Context) error {
	appName := app.NameFromContext(ctx)
	client := client.FromContext(ctx).API()

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	ms, err := client.ListMachines(ctx, app.ID, "started")
	if err != nil {
		return err
	}

	flaps, err := machines.NewFlapsClient(ctx, app)
	if err != nil {
		return err
	}

	for _, machine := range ms {
		resp, err := flaps.Get(machine)
		if err != nil {
			return err
		}

		fmt.Printf("Machine: %s", string(resp))
	}

	return err
}
