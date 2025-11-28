package ips

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/fly-go"
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
	egressIpAddresses, err := client.GetAppScopedEgressIPAddresses(ctx, appName)
	if err != nil {
		return err
	}

	// "Merge" everything into ipAddresses, not ideal, but we want to render everything in one table
	for _, v := range egressIpAddresses {
		for _, ip := range v {
			ipAddresses = append(ipAddresses, fly.IPAddress{
				ID:      ip.ID,
				Address: ip.IP,
				Type:    fmt.Sprintf("egress_v%d", ip.Version),
				Region:  ip.Region,
				// Use UpdatedAt as CreatedAt because for egress IPs that records when the IP's ownership last changed
				CreatedAt: ip.UpdatedAt,
			})
		}
	}

	if cfg.JSONOutput {
		return render.JSON(out, ipAddresses)
	}

	renderListTable(ctx, ipAddresses)
	SanityCheckAppScopedEgressIps(ctx, nil, egressIpAddresses, nil, "")
	fmt.Println("Learn more about Fly.io public, private, shared and dedicated IP addresses in our docs: https://fly.io/docs/networking/services/")
	return nil
}
