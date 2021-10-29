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

	cmd := BuildCommandKS(nil, nil, platformStrings, client, requireAppName)

	regionsStrings := docstrings.Get("platform.regions")
	BuildCommandKS(cmd, runPlatformRegions, regionsStrings, client, requireSession)

	vmSizesStrings := docstrings.Get("platform.vmsizes")
	BuildCommandKS(cmd, runPlatformVMSizes, vmSizesStrings, client, requireSession)

	statusStrings := docstrings.Get("platform.status")
	BuildCommandKS(cmd, runPlatformStatus, statusStrings, client, requireSession, requireAppName)

	return cmd
}

func runPlatformRegions(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	regions, _, err := cmdCtx.Client.API().PlatformRegions(ctx)
	if err != nil {
		return err
	}

	return cmdCtx.Frender(cmdctx.PresenterOption{
		Presentable: &presenters.Regions{Regions: regions},
	})
}

func runPlatformVMSizes(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	sizes, err := cmdCtx.Client.API().PlatformVMSizes(ctx)
	if err != nil {
		return err
	}

	return cmdCtx.Frender(cmdctx.PresenterOption{
		Presentable: &presenters.VMSizes{VMSizes: sizes},
	})
}

func runPlatformStatus(_ *cmdctx.CmdContext) error {
	docsURL := "https://status.fly.io/"
	fmt.Println("Opening", docsURL)
	return open.Run(docsURL)
}
