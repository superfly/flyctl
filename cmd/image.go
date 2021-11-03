package cmd

import (
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/cmdfmt"
)

func newImageCommand(client *client.Client) *Command {
	domainsStrings := docstrings.Get("image")
	cmd := BuildCommandKS(nil, nil, domainsStrings, client, requireSession)
	cmd.Aliases = []string{"img"}

	showStrings := docstrings.Get("image.show")
	BuildCommandKS(cmd, runImageShow, showStrings, client, requireSession, requireAppNameAsArg)

	updateStrings := docstrings.Get("image.update")
	updateCmd := BuildCommandKS(cmd, runImageUpdate, updateStrings, client, requireSession, requireAppNameAsArg)
	updateCmd.AddBoolFlag(BoolFlagOpts{
		Name:        "detach",
		Description: "Return immediately instead of monitoring update progress",
	})
	return cmd
}

func runImageUpdate(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	appName := cmdCtx.AppName

	app, err := cmdCtx.Client.API().GetImageInfo(ctx, appName)
	if err != nil {
		return err
	}

	if !app.ImageVersionTrackingEnabled {
		return fmt.Errorf("Image is not eligible for automated image updates.")
	}

	if !app.ImageUpgradeAvailable {
		return fmt.Errorf("Image is already running the latest image.")
	}

	cI := app.ImageDetails
	lI := app.LatestImageDetails

	current := fmt.Sprintf("%s:%s", cI.Repository, cI.Tag)
	target := fmt.Sprintf("%s:%s", lI.Repository, lI.Tag)

	if cI.Version != "" {
		current = fmt.Sprintf("%s %s", current, cI.Version)
	}

	if lI.Version != "" {
		target = fmt.Sprintf("%s %s", target, lI.Version)
	}

	confirm := false
	prompt := &survey.Confirm{
		Message: fmt.Sprintf("Update `%s` from %s to %s?", appName, current, target),
	}
	err = survey.AskOne(prompt, &confirm)
	if err != nil {
		return err
	}

	if !confirm {
		return nil
	}

	input := api.DeployImageInput{
		AppID:    appName,
		Image:    fmt.Sprintf("%s:%s", lI.Repository, lI.Tag),
		Strategy: api.StringPointer("ROLLING"),
	}

	release, releaseCommand, err := cmdCtx.Client.API().DeployImage(ctx, input)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmdCtx.Out, "Release v%d created\n", release.Version)
	if releaseCommand != nil {
		fmt.Fprintf(cmdCtx.Out, "Release command detected: this new release will not be available until the command succeeds.\n")
	}

	fmt.Println()
	cmdCtx.Status("updating", cmdctx.SDETAIL, "You can detach the terminal anytime without stopping the update")

	if releaseCommand != nil {
		cmdfmt.PrintBegin(cmdCtx.Out, "Release command")
		fmt.Printf("Command: %s\n", releaseCommand.Command)

		err = watchReleaseCommand(ctx, cmdCtx, cmdCtx.Client.API(), releaseCommand.ID)
		if err != nil {
			return err
		}
	}

	return watchDeployment(ctx, cmdCtx)

}

func runImageShow(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	appName := cmdCtx.AppName

	app, err := cmdCtx.Client.API().GetImageInfo(ctx, appName)
	if err != nil {
		return err
	}

	if app.ImageVersionTrackingEnabled && app.ImageUpgradeAvailable {
		current := fmt.Sprintf("%s:%s", app.ImageDetails.Repository, app.ImageDetails.Tag)
		latest := fmt.Sprintf("%s:%s", app.LatestImageDetails.Repository, app.LatestImageDetails.Tag)

		if app.ImageDetails.Version != "" {
			current = fmt.Sprintf("%s %s", current, app.ImageDetails.Version)
		}

		if app.LatestImageDetails.Version != "" {
			latest = fmt.Sprintf("%s %s", latest, app.LatestImageDetails.Version)
		}

		fmt.Fprintln(os.Stderr, aurora.Yellow(fmt.Sprintf("Update available! (%s -> %s)", current, latest)))
		fmt.Fprintln(os.Stderr, aurora.Yellow(fmt.Sprintf("Run `fly image update` to migrate to the latest image version.\n")))
	}

	err = cmdCtx.Frender(cmdctx.PresenterOption{
		Presentable: &presenters.ImageDetails{
			ImageDetails:    app.ImageDetails,
			TrackingEnabled: app.ImageVersionTrackingEnabled,
		}, HideHeader: true,
		Vertical: true,
		Title:    "Current image details",
	})
	if err != nil {
		return err
	}

	if app.ImageUpgradeAvailable {
		err = cmdCtx.Frender(cmdctx.PresenterOption{
			Presentable: &presenters.ImageDetails{
				ImageDetails:    app.LatestImageDetails,
				TrackingEnabled: app.ImageVersionTrackingEnabled,
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
