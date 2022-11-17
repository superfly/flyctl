package info

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
		long  = `Commands for managing IP addresses associated with an application`
		short = `Manage IP addresses for apps`
	)

	cmd := command.New("info", short, long, runInfo,
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

func runInfo(ctx context.Context) error {
	var (
		client  = client.FromContext(ctx).API()
		appName = app.NameFromContext(ctx)
	)

	appInfo, err := client.GetAppInfo(ctx, appName)
	if err != nil {
		return err
	}

	if appInfo.PlatformVersion == "machines" {
		return showMachineInfo(ctx, appName)
	} else {
		return showNomadInfo(ctx, appInfo)
	}
}
