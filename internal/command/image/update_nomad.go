package image

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"
)

func updateImageForNomad(ctx context.Context) error {
	var (
		client  = client.FromContext(ctx).API()
		io      = iostreams.FromContext(ctx)
		appName = app.NameFromContext(ctx)
	)

	app, err := client.GetImageInfo(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed to get image info: %w", err)
	}

	if !app.ImageVersionTrackingEnabled {
		return errors.New("image is not eligible for automated image updates")
	}

	if !app.ImageUpgradeAvailable {
		return errors.New("image is already running the latest image")
	}

	cI := app.ImageDetails
	lI := app.LatestImageDetails

	current := cI.ImageRef()
	target := lI.ImageRef()

	if cI.Version != "" {
		current = fmt.Sprintf("%s %s", current, cI.Version)
	}

	if lI.Version != "" {
		target = fmt.Sprintf("%s %s", target, lI.Version)
	}

	if !flag.GetYes(ctx) {
		switch confirmed, err := prompt.Confirmf(ctx, "Update `%s` from %s to %s?", appName, current, target); {
		case err == nil:
			if !confirmed {
				return nil
			}
		case prompt.IsNonInteractive(err):
			return prompt.NonInteractiveError("yes flag must be specified when not running interactively")
		default:
			return err
		}
	}

	input := api.DeployImageInput{
		AppID:    appName,
		Image:    lI.ImageRef(),
		Strategy: api.StringPointer("ROLLING"),
	}

	// Set the deployment strategy
	if val := flag.GetString(ctx, "strategy"); val != "" {
		input.Strategy = api.StringPointer(strings.ReplaceAll(strings.ToUpper(val), "-", "_"))
	}

	release, releaseCommand, err := client.DeployImage(ctx, input)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Release v%d created\n", release.Version)
	if releaseCommand != nil {
		fmt.Fprintln(io.Out, "Release command detected: this new release will not be available until the command succeeds.")
	}

	fmt.Fprintln(io.Out)

	tb := render.NewTextBlock(ctx)

	tb.Detail("You can detach the terminal anytime without stopping the update")

	if releaseCommand != nil {
		// TODO: don't use text block here
		tb := render.NewTextBlock(ctx, fmt.Sprintf("Release command detected: %s\n", releaseCommand.Command))
		tb.Done("This release will not be available until the release command succeeds.")

		if err := watch.ReleaseCommand(ctx, appName, releaseCommand.ID); err != nil {
			return err
		}

		release, err = client.GetAppRelease(ctx, appName, release.ID)
		if err != nil {
			return err
		}
	}
	return watch.Deployment(ctx, appName, release.EvaluationID)
}
