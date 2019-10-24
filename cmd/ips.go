package cmd

import (
	"fmt"
	"net"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
)

func newIPAddressesCommand() *Command {
	cmd := &Command{
		Command: &cobra.Command{
			Use:   "ips",
			Short: "manage ip addresses",
			Long:  "manage ip addresses",
		},
	}

	BuildCommand(cmd, runIPAddressesList, "list", "list ip addresses", os.Stdout, true, requireAppName)
	BuildCommand(cmd, runAllocateIPAddressV4, "allocate-v4", "allocate an IPv4 address", os.Stdout, true, requireAppName)
	BuildCommand(cmd, runAllocateIPAddressV6, "allocate-v6", "allocate an IPv6 address", os.Stdout, true, requireAppName)
	release := BuildCommand(cmd, runReleaseIPAddress, "release [ADDRESS]", "release an IP address", os.Stdout, true, requireAppName)
	release.Args = cobra.ExactArgs(1)

	return cmd
}

func runIPAddressesList(ctx *CmdContext) error {
	ipAddresses, err := ctx.FlyClient.GetIPAddresses(ctx.AppName())
	if err != nil {
		return err
	}

	return ctx.RenderView(PresenterOption{
		Presentable: &presenters.IPAddresses{IPAddresses: ipAddresses},
	})
}

func runAllocateIPAddressV4(ctx *CmdContext) error {
	return runAllocateIPAddress(ctx, "v4")
}

func runAllocateIPAddressV6(ctx *CmdContext) error {
	return runAllocateIPAddress(ctx, "v6")
}

func runAllocateIPAddress(ctx *CmdContext, addrType string) error {
	appName := ctx.AppName()

	ipAddress, err := ctx.FlyClient.AllocateIPAddress(appName, addrType)
	if err != nil {
		return err
	}

	return ctx.RenderView(PresenterOption{
		Presentable: &presenters.IPAddresses{IPAddresses: []api.IPAddress{*ipAddress}},
	})
}

func runReleaseIPAddress(ctx *CmdContext) error {
	appName := ctx.AppName()
	address := ctx.Args[0]

	if ip := net.ParseIP(address); ip == nil {
		return fmt.Errorf("Invalid IP address: '%s'", address)
	}

	ipAddress, err := ctx.FlyClient.FindIPAddress(appName, address)
	if err != nil {
		return err
	}

	if err := ctx.FlyClient.ReleaseIPAddress(ipAddress.ID); err != nil {
		return err
	}

	fmt.Printf("Released %s from %s\n", ipAddress.Address, appName)

	return nil
}
