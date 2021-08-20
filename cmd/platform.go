package cmd

import (
	"fmt"

	"github.com/skratchdot/open-golang/open"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/client"

	"github.com/superfly/flyctl/docstrings"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newPlatformCommand(client *client.Client) *Command {
	platformStrings := docstrings.Get("platform")

	cmd := BuildCommandKS(nil, nil, platformStrings, client, nil, requireAppName)

	regionsStrings := docstrings.Get("platform.regions")
	BuildCommandKS(cmd, runPlatformRegions, regionsStrings, client, nil, requireSession)

	vmSizesStrings := docstrings.Get("platform.vmsizes")
	BuildCommandKS(cmd, runPlatformVMSizes, vmSizesStrings, client, nil, requireSession)

	statusStrings := docstrings.Get("platform.status")
	BuildCommandKS(cmd, runPlatformStatus, statusStrings, client, nil, requireSession, requireAppName)

	return cmd
}

func runPlatformRegions(ctx *cmdctx.CmdContext) error {
	regions, _, err := ctx.Client.API().PlatformRegions()
	if err != nil {
		return err
	}

	return ctx.Frender(cmdctx.PresenterOption{
		Presentable: &presenters.Regions{Regions: regions},
	})
}

func runPlatformVMSizes(ctx *cmdctx.CmdContext) error {
	sizes, err := ctx.Client.API().PlatformVMSizes()
	if err != nil {
		return err
	}

	return ctx.Frender(cmdctx.PresenterOption{
		Presentable: &presenters.VMSizes{VMSizes: sizes},
	})
}

func runPlatformStatus(ctx *cmdctx.CmdContext) error {
	docsURL := "https://status.fly.io/"
	fmt.Println("Opening", docsURL)
	return open.Run(docsURL)
}
