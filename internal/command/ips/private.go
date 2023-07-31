package ips

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
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

	apiClient := client.FromContext(ctx).API()
	flapsClient, err := flaps.NewFromAppName(ctx, appName)
	if err != nil {
		return err
	}

	appstatus, err := apiClient.GetAppStatus(ctx, appName, false)
	if err != nil {
		return err
	}

	switch appstatus.PlatformVersion {
	case appconfig.NomadPlatform:
		_, backupRegions, err := apiClient.ListAppRegions(ctx, appName)
		if err != nil {
			return err
		}

		out := iostreams.FromContext(ctx).Out
		if conf := config.FromContext(ctx); conf.JSONOutput {
			_ = render.JSON(out, appstatus.Allocations)
			return nil
		}

		renderPrivateTable(ctx, appstatus.Allocations, backupRegions)
	case appconfig.MachinesPlatform:
		machines, _, err := flapsClient.ListFlyAppsMachines(ctx)
		if err != nil {
			return err
		}
		renderPrivateTableMachines(ctx, machines)
	}

	return nil
}
