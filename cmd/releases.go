package cmd

import (
	"os"

	"github.com/superfly/flyctl/docstrings"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppReleasesListCommand() *Command {
	releasesStrings := docstrings.Get("releases")
	cmd := BuildCommand(nil, runAppReleasesList, releasesStrings.Usage, releasesStrings.Short, releasesStrings.Long, true, os.Stdout, requireAppName)
	return cmd
}

func runAppReleasesList(ctx *CmdContext) error {
	releases, err := ctx.FlyClient.GetAppReleases(ctx.AppName, 25)
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.Releases{Releases: releases})
}
