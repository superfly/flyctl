package cmd

import (
	"fmt"
	"net"

	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/client"

	"github.com/superfly/flyctl/docstrings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
)

func newIPAddressesCommand(client *client.Client) *Command {

	ipsStrings := docstrings.Get("ips")
	cmd := BuildCommandKS(nil, nil, ipsStrings, client, requireSession, requireAppName)

	ipsListStrings := docstrings.Get("ips.list")
	BuildCommandKS(cmd, runIPAddressesList, ipsListStrings, client, requireSession, requireAppName)

	ipsPrivateListStrings := docstrings.Get("ips.private")
	BuildCommandKS(cmd, runPrivateIPAddressesList, ipsPrivateListStrings, client, requireSession, requireAppName)

	ipsAllocateV4Strings := docstrings.Get("ips.allocate-v4")
	allocateV4Command := BuildCommandKS(cmd, runAllocateIPAddressV4, ipsAllocateV4Strings, client, requireSession, requireAppName)

	allocateV4Command.AddStringFlag(StringFlagOpts{
		Name:        "region",
		Description: "The region where the address should be allocated",
	})

	ipsAllocateV6Strings := docstrings.Get("ips.allocate-v6")
	allocateV6Command := BuildCommandKS(cmd, runAllocateIPAddressV6, ipsAllocateV6Strings, client, requireSession, requireAppName)

	allocateV6Command.AddStringFlag(StringFlagOpts{
		Name:        "region",
		Description: "The region where the address should be allocated.",
	})

	allocateV6Command.AddBoolFlag(BoolFlagOpts{
		Name:        "private",
		Description: "Allocate a private ipv6 address",
	})

	ipsReleaseStrings := docstrings.Get("ips.release")
	release := BuildCommandKS(cmd, runReleaseIPAddress, ipsReleaseStrings, client, requireSession, requireAppName)
	release.Args = cobra.ExactArgs(1)

	return cmd
}

func runIPAddressesList(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	ipAddresses, err := cmdCtx.Client.API().GetIPAddresses(ctx, cmdCtx.AppName)
	if err != nil {
		return err
	}

	return cmdCtx.Frender(cmdctx.PresenterOption{
		Presentable: &presenters.IPAddresses{IPAddresses: ipAddresses},
	})
}

func runAllocateIPAddressV4(ctx *cmdctx.CmdContext) error {
	return runAllocateIPAddress(ctx, "v4")
}

func runAllocateIPAddressV6(ctx *cmdctx.CmdContext) error {
	private := ctx.Config.GetBool("private")
	if private {
		return runAllocateIPAddress(ctx, "private_v6")
	}
	return runAllocateIPAddress(ctx, "v6")
}

func runAllocateIPAddress(cmdCtx *cmdctx.CmdContext, addrType string) error {
	ctx := cmdCtx.Command.Context()

	appName := cmdCtx.AppName
	regionCode := cmdCtx.Config.GetString("region")

	ipAddress, err := cmdCtx.Client.API().AllocateIPAddress(ctx, appName, addrType, regionCode)
	if err != nil {
		return err
	}

	return cmdCtx.Frender(cmdctx.PresenterOption{
		Presentable: &presenters.IPAddresses{IPAddresses: []api.IPAddress{*ipAddress}},
	})
}

func runReleaseIPAddress(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	appName := cmdCtx.AppName
	address := cmdCtx.Args[0]

	if ip := net.ParseIP(address); ip == nil {
		return fmt.Errorf("Invalid IP address: '%s'", address)
	}

	ipAddress, err := cmdCtx.Client.API().FindIPAddress(ctx, appName, address)
	if err != nil {
		return err
	}

	if err := cmdCtx.Client.API().ReleaseIPAddress(ctx, ipAddress.ID); err != nil {
		return err
	}

	fmt.Printf("Released %s from %s\n", ipAddress.Address, appName)

	return nil
}

func runPrivateIPAddressesList(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	appstatus, err := cmdCtx.Client.API().GetAppStatus(ctx, cmdCtx.AppName, false)
	if err != nil {
		return err
	}

	_, backupRegions, err := cmdCtx.Client.API().ListAppRegions(ctx, cmdCtx.AppName)

	if err != nil {
		return err
	}

	if cmdCtx.OutputJSON() {
		cmdCtx.WriteJSON(appstatus.Allocations)
		return nil
	}

	table := helpers.MakeSimpleTable(cmdCtx.Out, []string{"ID", "Region", "IP"})

	for _, alloc := range appstatus.Allocations {

		region := alloc.Region
		if len(backupRegions) > 0 {
			for _, r := range backupRegions {
				if alloc.Region == r.Code {
					region = alloc.Region + "(B)"
					break
				}
			}
		}

		table.Append([]string{alloc.IDShort, region, alloc.PrivateIP})
	}

	table.Render()

	return nil
}
