package ips

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newList() *cobra.Command {
	const (
		long  = `Lists the IP addresses allocated to the application`
		short = `List allocated IP addresses`
	)

	cmd := command.New("list", short, long, runIPAddressesList,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)
	return cmd
}

func runIPAddressesList(ctx context.Context) error {
	client := client.FromContext(ctx).API()

	appName := app.NameFromContext(ctx)
	ipAddresses, err := client.GetIPAddresses(ctx, appName)
	if err != nil {
		return err
	}

	renderListTable(ctx, ipAddresses)
	return nil
}
