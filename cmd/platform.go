package cmd

import (
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/client"

	"github.com/superfly/flyctl/docstrings"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newPlatformCommand(client *client.Client) *Command {
	platformStrings := docstrings.Get("platform")

	cmd := BuildCommandKS(nil, nil, platformStrings, client, requireAppName)

	return cmd
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
