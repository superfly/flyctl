package ips

import (
	"context"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
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
		flag.Yes(),
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
		flag.String{
			Name:        "network",
			Description: "Target network name for a Flycast private IPv6 address",
		},
	)

	return cmd
}

func runAllocateIPAddressV4(ctx context.Context) error {
	addrType := "v4"
	if flag.GetBool(ctx, "shared") {
		addrType = "shared_v4"
	} else if !flag.GetBool(ctx, "yes") {
		msg := `Looks like you're accessing a paid feature. Dedicated IPv4 addresses now cost $2/mo.
Are you ok with this? Alternatively, you could allocate a shared IPv4 address with the --shared flag.`

		switch confirmed, err := prompt.Confirm(ctx, msg); {
		case err == nil:
			if !confirmed {
				return nil
			}
		case prompt.IsNonInteractive(err):
			return prompt.NonInteractiveError("yes flag must be specified when not running interactively")
		default:
			return err
		}
	}
	return runAllocateIPAddress(ctx, addrType, nil, "")
}

func runAllocateIPAddressV6(ctx context.Context) (err error) {
	private := flag.GetBool(ctx, "private")
	if private {
		orgSlug := flag.GetOrg(ctx)
		var org *fly.Organization

		if orgSlug != "" {
			org, err = orgs.OrgFromSlug(ctx, orgSlug)
			if err != nil {
				return err
			}
		}

		network := flag.GetString(ctx, "network")

		return runAllocateIPAddress(ctx, "private_v6", org, network)
	}

	return runAllocateIPAddress(ctx, "v6", nil, "")
}

func runAllocateIPAddress(ctx context.Context, addrType string, org *fly.Organization, network string) (err error) {
	client := flyutil.ClientFromContext(ctx)

	appName := appconfig.NameFromContext(ctx)

	if addrType == "shared_v4" {
		ip, err := client.AllocateSharedIPAddress(ctx, appName)
		if err != nil {
			return err
		}

		renderSharedTable(ctx, ip)

		return nil
	}

	region := flag.GetRegion(ctx)

	ipAddress, err := client.AllocateIPAddress(ctx, appName, addrType, region, org, network)
	if err != nil {
		return err
	}

	ipAddresses := []fly.IPAddress{*ipAddress}
	renderListTable(ctx, ipAddresses)
	return nil
}
