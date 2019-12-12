package cmd

import (
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppReleasesListCommand() *Command {
	cmd := BuildCommand(nil, runAppReleasesList, "releases", "list app releases", os.Stdout, true, requireAppName)
	BuildCommand(cmd, runShowCurrentReleaseDetails, "latest", "show latest release", os.Stdout, true, requireAppName)
	show := BuildCommand(cmd, runShowReleaseDetails, "show [VERSION]", "show detailed release information", os.Stdout, true, requireAppName)
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
