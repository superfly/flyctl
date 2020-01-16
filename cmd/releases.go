package cmd

import (
	"github.com/superfly/flyctl/docstrings"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppReleasesListCommand() *Command {
	releasesStrings := docstrings.Get("releases")
	cmd := BuildCommand(nil, runAppReleasesList, releasesStrings.Usage, releasesStrings.Short, releasesStrings.Long, true, os.Stdout, requireAppName)

	releasesLatestStrings := docstrings.Get("releases.latest")
	BuildCommand(cmd, runShowCurrentReleaseDetails, releasesLatestStrings.Usage, releasesLatestStrings.Short, releasesLatestStrings.Long, true, os.Stdout, requireAppName)

	releasesShowStrings := docstrings.Get("releases.show")
	show := BuildCommand(cmd, runShowReleaseDetails, releasesShowStrings.Usage, releasesShowStrings.Short, releasesShowStrings.Long, true, os.Stdout, requireAppName)
	show.Args = cobra.ExactArgs(1)
	return cmd
}

func runAppReleasesList(ctx *CmdContext) error {
	releases, err := ctx.FlyClient.GetAppReleases(ctx.AppName, 25)
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.Releases{Releases: releases})
}

func runShowCurrentReleaseDetails(ctx *CmdContext) error {
	release, err := ctx.FlyClient.GetAppCurrentRelease(ctx.AppName)
	if err != nil {
		return err
	}

	return renderReleaseDetails(ctx, release)
}

func runShowReleaseDetails(ctx *CmdContext) error {
	versionArg := ctx.Args[0]
	if strings.HasPrefix(versionArg, "v") {
		versionArg = versionArg[1:]
	}
	version, err := strconv.Atoi(versionArg)
	if err != nil {
		return err
	}

	release, err := ctx.FlyClient.GetAppReleaseVersion(ctx.AppName, version)
	if err != nil {
		return err
	}

	return renderReleaseDetails(ctx, release)
}

func renderReleaseDetails(ctx *CmdContext, release *api.Release) error {
	return ctx.RenderView(
		PresenterOption{
			Presentable: &presenters.ReleaseDetails{Release: *release},
			Vertical:    true,
		},
		PresenterOption{
			Presentable: &presenters.DeploymentTaskStatus{Release: *release},
		},
	)
}
