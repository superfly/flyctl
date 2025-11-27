package ips

import (
	"context"
	"fmt"
	"slices"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
)

func confirmAlloc(ctx context.Context, msg string) error {
	// Skip all confirmation if the user provided a `--yes` flag
	if flag.GetBool(ctx, "yes") {
		return nil
	}

	switch confirmed, err := prompt.Confirm(ctx, msg); {
	case err == nil:
		if !confirmed {
			return fmt.Errorf("user aborted")
		}

		return nil
	case prompt.IsNonInteractive(err):
		return prompt.NonInteractiveError("yes flag must be specified when not running interactively")
	default:
		return err
	}
}

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

func newAllocateEgress() *cobra.Command {
	const (
		long  = `(Beta) Allocates a pair of egress IP addresses for an app`
		short = `(Beta) Allocate app-scoped egress IPs`
	)

	cmd := command.New("allocate-egress", short, long, runAllocateEgressIPAddresses,
		command.RequireSession,
		command.RequireAppName)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Region(),
		flag.Yes(),
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

		if err := confirmAlloc(ctx, msg); err != nil {
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

	orgID := ""
	if org != nil {
		orgID = org.ID
	}

	ipAddress, err := client.AllocateIPAddress(ctx, appName, addrType, region, orgID, network)
	if err != nil {
		return err
	}

	ipAddresses := []fly.IPAddress{*ipAddress}
	renderListTable(ctx, ipAddresses)
	return nil
}

func runAllocateEgressIPAddresses(ctx context.Context) (err error) {
	client := flyutil.ClientFromContext(ctx)
	flapsClient := flapsutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	region := flag.GetRegion(ctx)
	if region == "" {
		return fmt.Errorf("a region must be provided when allocating an app-scoped egress IP address")
	}

	if !flag.GetBool(ctx, "yes") {
		msg := `You are allocating an egress IP address. This type of IPs are used when your machine accesses an external resource, and cannot be used to access your app.
If you don't know what this is, you probably want to allocate an Anycast IP using allocate-v4 or allocate-v6 instead.
Please confirm that this is what you need.`

		if err := confirmAlloc(ctx, msg); err != nil {
			return err
		}

		machines, err := flapsClient.List(ctx, appName, "")

		if err == nil && !slices.ContainsFunc(machines, func(m *fly.Machine) bool {
			return m.Region == region
		}) {
			msg = fmt.Sprintf(`You are allocating a egress IP in region %s but your app has no machines there.
Only machines in the same region can make use of egress IPs in that region.
If this is intentional, type Y to continue.`, region)

			if err := confirmAlloc(ctx, msg); err != nil {
				return err
			}
		}
	}

	v4, v6, err := client.AllocateAppScopedEgressIPAddress(ctx, appName, region)
	if err != nil {
		return err
	}

	fmt.Printf("Allocated egress IPs for region %s:\n", region)
	fmt.Printf("%s\n", v4.String())
	fmt.Printf("%s\n", v6.String())
	fmt.Println("Newly-allocated egress IPs may need 5 - 10 minutes to take effect on existing machines.")

	sanityCheckAppScopedEgressIps(ctx, nil, nil)

	return nil
}
