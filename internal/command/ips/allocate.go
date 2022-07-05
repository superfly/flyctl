package ips

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newAllocatev4() *cobra.Command {
	const (
		long  = `Allocates an IPv4 address to the application`
		short = `Allocate an IPv4 address`
	)

	cmd := command.New("allocate-v4", short, long, runAllocateIPAddressV4,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Region(),
	)
	return cmd
}

func newAllocatev6() *cobra.Command {
	const (
		long  = `Allocates an IPv6 address to the application`
		short = `Allocate an IPv6 address`
	)

	cmd := command.New("allocate-v6", short, long, runAllocateIPAddressV6,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Region(),
		flag.Bool{
			Name:        "private",
			Description: "Allocate a private IPv6 address",
		},
	)

	return cmd
}

func runAllocateIPAddressV4(ctx context.Context) error {
	return runAllocateIPAddress(ctx, "v4")
}

func runAllocateIPAddressV6(ctx context.Context) error {
	private := flag.GetBool(ctx, "private")
	if private {
		return runAllocateIPAddress(ctx, "private_v6")
	}
	return runAllocateIPAddress(ctx, "v6")
}

func runAllocateIPAddress(ctx context.Context, addrType string) error {
	client := client.FromContext(ctx).API()

	appName := app.NameFromContext(ctx)
	region := flag.GetRegion(ctx)

	ipAddress, err := client.AllocateIPAddress(ctx, appName, addrType, region)
	if err != nil {
		return err
	}

	ipAddresses := []api.IPAddress{*ipAddress}
	renderListTable(ctx, ipAddresses)
	return nil
}
