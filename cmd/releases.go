package cmd

import (
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/client"

	"github.com/superfly/flyctl/docstrings"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newReleasesCommand(client *client.Client) *Command {
	releasesStrings := docstrings.Get("releases")
	cmd := BuildCommandKS(nil, runReleases, releasesStrings, client, requireSession, requireAppName)
	return cmd
}

func runReleases(cmdCtx *cmdctx.CmdContext) error {
	ctx := createCancellableContext()

	releases, err := cmdCtx.Client.API().GetAppReleases(ctx, cmdCtx.AppName, 25)
	if err != nil {
		return err
	}
	return cmdCtx.Render(&presenters.Releases{Releases: releases})
}
