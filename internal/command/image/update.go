package image

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/watch"
)

func newUpdate() *cobra.Command {
	const (
		long = `This will update the application's image to the latest available version.
The update will perform a rolling restart against each VM, which may result in a brief service disruption.`

		short = "Updates the app's image to the latest available version. (Fly Postgres only)"

		usage = "update"
	)

	cmd := command.New(usage, short, long, runUpdate,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Yes(),
		flag.Bool{
			Name:        "detach",
			Description: "Return immediately instead of monitoring update progress",
		},
	)

	return cmd
}

func runUpdate(ctx context.Context) error {
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

	current := fmt.Sprintf("%s:%s", cI.Repository, cI.Tag)
	target := fmt.Sprintf("%s:%s", lI.Repository, lI.Tag)

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
		Image:    fmt.Sprintf("%s:%s", lI.Repository, lI.Tag),
		Strategy: api.StringPointer("ROLLING"),
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

		if err := watch.ReleaseCommand(ctx, releaseCommand.ID); err != nil {
			return err
		}

		release, err = client.GetAppRelease(ctx, appName, release.ID)
		if err != nil {
			return err
		}
	}

	return watch.Deployment(ctx, release.EvaluationID)
}
