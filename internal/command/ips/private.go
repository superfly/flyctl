package ips

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newPrivate() *cobra.Command {
	const (
		long = `List instances private IP addresses, accessible from within the Fly network`
		short = `List instances private IP addresses`
	)

	cmd := command.New("private", short, long, runPrivateIPAddressesList,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)
	return cmd
}

func runPrivateIPAddressesList(ctx context.Context) error {
	client := client.FromContext(ctx).API()

	appName := app.NameFromContext(ctx)
	appstatus, err := client.GetAppStatus(ctx, appName, false)
	if err != nil {
		return err
	}

	_, backupRegions, err := client.ListAppRegions(ctx, appName)

	if err != nil {
		return err
	}

	out := iostreams.FromContext(ctx).Out
	if conf := config.FromContext(ctx); conf.JSONOutput {
		_ = render.JSON(out, appstatus.Allocations)
		return nil
	}

	renderPrivateTable(ctx, appstatus.Allocations, backupRegions)
	return nil
}
