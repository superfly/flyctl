package cmd

import (
	"fmt"
	"github.com/superfly/flyctl/cmdctx"
	"net"
	"os"

	"github.com/superfly/flyctl/docstrings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
)

func newIPAddressesCommand() *Command {

	ipsStrings := docstrings.Get("ips")
	cmd := &Command{
		Command: &cobra.Command{
			Use:   ipsStrings.Usage,
			Short: ipsStrings.Short,
			Long:  ipsStrings.Long,
		},
	}

	ipsListStrings := docstrings.Get("ips.list")
	BuildCommand(cmd, runIPAddressesList, ipsListStrings.Usage, ipsListStrings.Short, ipsListStrings.Long, os.Stdout, requireSession, requireAppName)

	ipsAllocateV4Strings := docstrings.Get("ips.allocate-v4")
	BuildCommand(cmd, runAllocateIPAddressV4, ipsAllocateV4Strings.Usage, ipsAllocateV4Strings.Short, ipsAllocateV4Strings.Long, os.Stdout, requireSession, requireAppName)

	ipsAllocateV6Strings := docstrings.Get("ips.allocate-v6")
	BuildCommand(cmd, runAllocateIPAddressV6, ipsAllocateV6Strings.Usage, ipsAllocateV6Strings.Short, ipsAllocateV6Strings.Long, os.Stdout, requireSession, requireAppName)

	ipsReleaseStrings := docstrings.Get("ips.release")
	release := BuildCommand(cmd, runReleaseIPAddress, ipsReleaseStrings.Usage, ipsReleaseStrings.Short, ipsReleaseStrings.Long, os.Stdout, requireSession, requireAppName)
	release.Args = cobra.ExactArgs(1)

	return cmd
}

func runIPAddressesList(ctx *cmdctx.CmdContext) error {
	ipAddresses, err := ctx.Client.API().GetIPAddresses(ctx.AppName)
	if err != nil {
		return err
	}

	return ctx.Frender(ctx.Out, cmdctx.PresenterOption{
		Presentable: &presenters.IPAddresses{IPAddresses: ipAddresses},
	})
}

func runAllocateIPAddressV4(ctx *cmdctx.CmdContext) error {
	return runAllocateIPAddress(ctx, "v4")
}

func runAllocateIPAddressV6(ctx *cmdctx.CmdContext) error {
	return runAllocateIPAddress(ctx, "v6")
}

func runAllocateIPAddress(ctx *cmdctx.CmdContext, addrType string) error {
	appName := ctx.AppName

	ipAddress, err := ctx.Client.API().AllocateIPAddress(appName, addrType)
	if err != nil {
		return err
	}

	return ctx.Frender(ctx.Out, cmdctx.PresenterOption{
		Presentable: &presenters.IPAddresses{IPAddresses: []api.IPAddress{*ipAddress}},
	})
}

func runReleaseIPAddress(ctx *cmdctx.CmdContext) error {
	appName := ctx.AppName
	address := ctx.Args[0]

	if ip := net.ParseIP(address); ip == nil {
		return fmt.Errorf("Invalid IP address: '%s'", address)
	}

	ipAddress, err := ctx.Client.API().FindIPAddress(appName, address)
	if err != nil {
		return err
	}

	if err := ctx.Client.API().ReleaseIPAddress(ipAddress.ID); err != nil {
		return err
	}

	fmt.Printf("Released %s from %s\n", ipAddress.Address, appName)

	return nil
}
