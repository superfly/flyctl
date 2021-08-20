package cmd

import (
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/client"

	"github.com/superfly/flyctl/docstrings"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newReleasesCommand(client *client.Client) *Command {
	releasesStrings := docstrings.Get("releases")
	cmd := BuildCommandKS(nil, runReleases, releasesStrings, client, nil, requireSession, requireAppName)
	return cmd
}

func runReleases(ctx *cmdctx.CmdContext) error {
	releases, err := ctx.Client.API().GetAppReleases(ctx.AppName, 25)
	if err != nil {
		return err
	}
	return ctx.Render(&presenters.Releases{Releases: releases})
}
