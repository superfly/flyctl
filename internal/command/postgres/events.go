package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
)

func newEvents() *cobra.Command {
	const (
		short = "Track major cluster events"
		long  = short + "\n"

		usage = "events"
	)

	cmd := command.New(usage, short, long, nil)

	cmd.AddCommand(
		newListEvents(),
	)

	flag.Add(cmd, flag.JSONOutput())
	return cmd
}

func newListEvents() *cobra.Command {
	const (
		short = "List major cluster events"
		long  = short + "\n"

		usage = "list"
	)

	cmd := command.New(usage, short, long, runListEvents,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "event",
			Shorthand:   "e",
			Description: "Event type in a postgres cluster",
		},
	)

	return cmd

}

func runListEvents(ctx context.Context) error {
	var (
		client  = client.FromContext(ctx).API()
		appName = appconfig.NameFromContext(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	if !app.IsPostgresApp() {
		return fmt.Errorf("app %s is not a postgres app", appName)
	}

	ctx, err = apps.BuildContext(ctx, app)
	if err != nil {
		return err
	}

	machines, err := mach.ListActive(ctx)
	if err != nil {
		return err
	}

	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}

	cmd, err := flypg.NewCommand(ctx, app)
	if err != nil {
		return err
	}

	err = cmd.ListEvents(ctx, leader.PrivateIP)
	if err != nil {
		return err
	}

	return nil
}
