package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagnames"
	"github.com/superfly/flyctl/internal/flyutil"
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
		short = "Outputs a formatted list of cluster events"
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
		flag.String{
			Name:        "limit",
			Shorthand:   "l",
			Description: "Set the maximum number of entries to output (default: 20)",
		},
		flag.String{
			Name:        "node-id",
			Shorthand:   "i",
			Description: "Restrict entries to node with this ID",
		},
		flag.String{
			Name:        "node-name",
			Shorthand:   "n",
			Description: "Restrict entries to node with this name",
		},
		flag.Bool{
			Name:        "all",
			Shorthand:   "o",
			Description: "Outputs all entries",
		},
		flag.Bool{
			Name:        "compact",
			Shorthand:   "d",
			Description: "Omit the 'Details' column",
		},
	)

	return cmd
}

func runListEvents(ctx context.Context) error {
	var (
		client  = flyutil.ClientFromContext(ctx)
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

	if !IsFlex(leader) {
		return fmt.Errorf("this feature is not compatible with this postgres service ")
	}

	ignoreFlags := []string{
		flagnames.AccessToken, flagnames.App, flagnames.AppConfigFilePath,
		flagnames.Verbose, "help",
	}

	flagsName := flag.GetFlagsName(ctx, ignoreFlags)

	cmd, err := flypg.NewCommand(ctx, app)
	if err != nil {
		return err
	}

	err = cmd.ListEvents(ctx, leader.PrivateIP, flagsName)
	if err != nil {
		return err
	}

	return nil
}
