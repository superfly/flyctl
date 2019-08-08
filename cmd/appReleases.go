package cmd

import (
	"os"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppReleasesListCommand() *Command {
	return BuildCommand(nil, runAppReleasesList, "releases", "list app releases", os.Stdout, true, requireAppName)
}

func runAppReleasesList(ctx *CmdContext) error {
	releases, err := ctx.FlyClient.GetAppReleases(ctx.AppName(), 25)
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.Releases{Releases: releases})
}
