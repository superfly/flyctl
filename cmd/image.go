package cmd

import (
	"fmt"
	"os"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"
)

func newImageCommand(client *client.Client) *Command {
	domainsStrings := docstrings.Get("image")
	cmd := BuildCommandKS(nil, nil, domainsStrings, client, requireSession)
	cmd.Aliases = []string{"img"}

	showStrings := docstrings.Get("image.show")
	BuildCommandKS(cmd, runImageShow, showStrings, client, requireSession, requireAppNameAsArg)

	return cmd
}

func runImageShow(ctx *cmdctx.CmdContext) error {
	appName := ctx.AppName

	app, err := ctx.Client.API().GetImageInfo(appName)
	if err != nil {
		return err
	}

	if app.ImageVersionTrackingEnabled && app.ImageUpgradeAvailable {
		current := fmt.Sprintf("%s:%s %s", app.ImageDetails.Repository, app.ImageDetails.Tag, app.ImageDetails.Version)
		latest := fmt.Sprintf("%s:%s %s", app.LatestImageDetails.Repository, app.LatestImageDetails.Tag, app.LatestImageDetails.Version)
		fmt.Fprintln(os.Stderr, aurora.Yellow(fmt.Sprintf("Update available %s -> %s", current, latest)))
	}

	err = ctx.Frender(cmdctx.PresenterOption{
		Presentable: &presenters.ImageDetails{
			ImageDetails:    app.ImageDetails,
			TrackingEnabled: app.ImageVersionTrackingEnabled,
		}, HideHeader: true,
		Vertical: true,
		Title:    "Image details",
	})
	if err != nil {
		return err
	}

	if app.ImageUpgradeAvailable {
		err = ctx.Frender(cmdctx.PresenterOption{
			Presentable: &presenters.ImageDetails{
				ImageDetails: app.LatestImageDetails,
			},
			HideHeader: true,
			Vertical:   true,
			Title:      "Latest image details",
		})
		if err != nil {
			return err
		}
	}

	return nil
}
