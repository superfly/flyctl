package cmd

import (
	"os"

	"github.com/superfly/flyctl/cmdctx"

	"github.com/superfly/flyctl/docstrings"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newReleasesCommand() *Command {
	releasesStrings := docstrings.Get("releases")
	cmd := BuildCommand(nil, runReleases, releasesStrings.Usage, releasesStrings.Short, releasesStrings.Long, os.Stdout, requireSession, requireAppName)
	return cmd
}

func runReleases(ctx *cmdctx.CmdContext) error {
	releases, err := ctx.Client.API().GetAppReleases(ctx.AppName, 25)
	if err != nil {
		return err
	}
	return ctx.Render(&presenters.Releases{Releases: releases})
}
