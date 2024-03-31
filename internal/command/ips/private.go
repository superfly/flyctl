package ips

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
)

func newPrivate() *cobra.Command {
	const (
		long  = `List instances private IP addresses, accessible from within the Fly network`
		short = `List instances private IP addresses`
	)

	cmd := command.New("private", short, long, runPrivateIPAddressesList,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
	)
	return cmd
}

func runPrivateIPAddressesList(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return err
	}

	machines, _, err := flapsClient.ListFlyAppsMachines(ctx)
	if err != nil {
		return err
	}
	renderPrivateTableMachines(ctx, machines)

	return nil
}
