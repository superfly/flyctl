package services

import (
	"context"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"

	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	const (
		long  = `Shows information about the services of the application.`
		short = `Show the application's services`
	)

	cmd := command.New("services", short, long, runServiceInfo,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runServiceInfo(ctx context.Context) error {
	var (
		client  = client.FromContext(ctx).API()
		appName = app.NameFromContext(ctx)
	)

	appInfo, err := client.GetAppInfo(ctx, appName)
	if err != nil {
		return err
	}

	if appInfo.PlatformVersion == "machines" {
		return showMachineServiceInfo(ctx, appInfo)
	} else {
		return showNomadServiceInfo(ctx, appInfo)
	}
}
