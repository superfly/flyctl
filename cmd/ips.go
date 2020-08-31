package cmd

import (
	"fmt"
	"net"
	"os"

	"github.com/superfly/flyctl/cmdctx"

	"github.com/superfly/flyctl/docstrings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
)

func newIPAddressesCommand() *Command {

	ipsStrings := docstrings.Get("ips")
	cmd := BuildCommandKS(nil, nil, ipsStrings, os.Stdout, requireSession, requireAppName)

	ipsListStrings := docstrings.Get("ips.list")
	BuildCommandKS(cmd, runIPAddressesList, ipsListStrings, os.Stdout, requireSession, requireAppName)

	ipsAllocateV4Strings := docstrings.Get("ips.allocate-v4")
	BuildCommandKS(cmd, runAllocateIPAddressV4, ipsAllocateV4Strings, os.Stdout, requireSession, requireAppName)

	ipsAllocateV6Strings := docstrings.Get("ips.allocate-v6")
	BuildCommandKS(cmd, runAllocateIPAddressV6, ipsAllocateV6Strings, os.Stdout, requireSession, requireAppName)

	ipsReleaseStrings := docstrings.Get("ips.release")
	release := BuildCommandKS(cmd, runReleaseIPAddress, ipsReleaseStrings, os.Stdout, requireSession, requireAppName)
	release.Args = cobra.ExactArgs(1)

	return cmd
}

func runIPAddressesList(commandContext *cmdctx.CmdContext) error {
	ipAddresses, err := commandContext.Client.API().GetIPAddresses(commandContext.AppName)
	if err != nil {
		return err
	}

	return commandContext.Frender(cmdctx.PresenterOption{
		Presentable: &presenters.IPAddresses{IPAddresses: ipAddresses},
	})
}

func runAllocateIPAddressV4(ctx *cmdctx.CmdContext) error {
	return runAllocateIPAddress(ctx, "v4")
}

func runAllocateIPAddressV6(ctx *cmdctx.CmdContext) error {
	return runAllocateIPAddress(ctx, "v6")
}

func runAllocateIPAddress(commandContext *cmdctx.CmdContext, addrType string) error {
	appName := commandContext.AppName

	ipAddress, err := commandContext.Client.API().AllocateIPAddress(appName, addrType)
	if err != nil {
		return err
	}

	return commandContext.Frender(cmdctx.PresenterOption{
		Presentable: &presenters.IPAddresses{IPAddresses: []api.IPAddress{*ipAddress}},
	})
}

func runReleaseIPAddress(commandContext *cmdctx.CmdContext) error {
	appName := commandContext.AppName
	address := commandContext.Args[0]

	if ip := net.ParseIP(address); ip == nil {
		return fmt.Errorf("Invalid IP address: '%s'", address)
	}

	ipAddress, err := commandContext.Client.API().FindIPAddress(appName, address)
	if err != nil {
		return err
	}

	if err := commandContext.Client.API().ReleaseIPAddress(ipAddress.ID); err != nil {
		return err
	}

	fmt.Printf("Released %s from %s\n", ipAddress.Address, appName)

	return nil
}
