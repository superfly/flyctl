package ips

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
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
		flag.Bool{
			Name:        "shared",
			Description: "Allocates a shared IPv4",
			Default:     false,
		},
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
		flag.Org(),
	)

	return cmd
}

func runAllocateIPAddressV4(ctx context.Context) error {
	addrType := "v4"
	if flag.GetBool(ctx, "shared") {
		addrType = "shared_v4"
	}
	return runAllocateIPAddress(ctx, addrType, nil)
}

func runAllocateIPAddressV6(ctx context.Context) (err error) {
	private := flag.GetBool(ctx, "private")
	if private {
		orgSlug := flag.GetOrg(ctx)
		var org *api.Organization

		if orgSlug != "" {
			org, err = orgs.OrgFromSlug(ctx, orgSlug)
			if err != nil {
				return err
			}
		}
		return runAllocateIPAddress(ctx, "private_v6", org)
	}

	return runAllocateIPAddress(ctx, "v6", nil)
}

func runAllocateIPAddress(ctx context.Context, addrType string, org *api.Organization) (err error) {
	client := client.FromContext(ctx).API()

	appName := app.NameFromContext(ctx)

	if addrType == "shared_v4" {
		ip, err := client.AllocateSharedIPAddress(ctx, appName)
		if err != nil {
			return err
		}

		renderSharedTable(ctx, ip)

		return nil
	}

	region := flag.GetRegion(ctx)

	ipAddress, err := client.AllocateIPAddress(ctx, appName, addrType, region, org)
	if err != nil {
		return err
	}

	ipAddresses := []api.IPAddress{*ipAddress}
	renderListTable(ctx, ipAddresses)
	return nil
}
