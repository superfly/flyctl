package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/pkg/flaps"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newUpdate() (cmd *cobra.Command) {
	const (
		long = `Performs a rolling upgrade against the target Postgres cluster.
`
		short = "Updates the Postgres cluster to the latest eligible version"
		usage = "update"
	)

	cmd = command.New(usage, short, long, runUpdate,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return
}

func runUpdate(ctx context.Context) error {
	appName := app.NameFromContext(ctx)
	client := client.FromContext(ctx).API()
	io := iostreams.FromContext(ctx)

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	cli, err := flaps.New(ctx, app)
	if err != nil {
		return err
	}

	machines, err := client.ListMachines(ctx, app.ID, "started")
	if err != nil {
		return err
	}

	// imageRef, err := client.GetLatestImageTag(ctx, "flyio/postgres")
	// if err != nil {
	// 	return err
	// }

	// TODO - Once role/failover has been converted to http endpoints we can
	// work to orchestrate this a bit better.
	for _, machine := range machines {
		fmt.Fprintf(io.Out, "Update machine %q... ", machine.ID)
		fmt.Fprintf(io.Out, "Current metadata: %+v", machine.Config.Metadata)

		machineConf := machine.Config
		machineConf.Image = "flyio/postgres:14.2"

		input := api.LaunchMachineInput{
			ID:      machine.ID,
			AppID:   app.ID,
			OrgSlug: machine.App.Organization.Slug,
			Region:  "dev",
			Config:  &machineConf,
		}

		fmt.Printf("Machine config: %+v", input.Config)

		resp, err := cli.Update(ctx, input)
		if err != nil {
			return err
		}

		fmt.Printf("Response: %s", resp)
	}

	return nil
}
