package ips

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
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

	cmd.Aliases = []string{"ls"}

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
	)
	return cmd
}

func runIPAddressesList(ctx context.Context) error {
	cfg := config.FromContext(ctx)
	client := flyutil.ClientFromContext(ctx)
	out := iostreams.FromContext(ctx).Out

	appName := appconfig.NameFromContext(ctx)
	ipAddresses, err := client.GetIPAddresses(ctx, appName)
	if err != nil {
		return err
	}

	if cfg.JSONOutput {
		return render.JSON(out, ipAddresses)
	}

	renderListTable(ctx, ipAddresses)
	fmt.Println("Learn more about Fly.io public, private, shared and dedicated IP addresses in our docs: https://fly.io/docs/networking/services/")
	return nil
}
