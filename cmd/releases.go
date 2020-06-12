package cmd

import (
	"github.com/superfly/flyctl/cmdctx"
	"os"

	"github.com/superfly/flyctl/docstrings"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppReleasesListCommand() *Command {
	releasesStrings := docstrings.Get("releases")
	cmd := BuildCommand(nil, runAppReleasesList, releasesStrings.Usage, releasesStrings.Short, releasesStrings.Long, os.Stdout, requireSession, requireAppName)
	return cmd
}

func runAppReleasesList(ctx *cmdctx.CmdContext) error {
	releases, err := ctx.Client.API().GetAppReleases(ctx.AppName, 25)
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.Releases{Releases: releases})
}
