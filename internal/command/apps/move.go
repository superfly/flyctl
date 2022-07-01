package apps

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/prompt"
)

func newMove() *cobra.Command {
	const (
		long = `The APPS MOVE command will move an application to another
organization the current user belongs to.
`
		short = "Move an app to another organization"
		usage = "move [APPNAME]"
	)

	move := command.New(usage, short, long, RunMove,
		command.RequireSession,
	)

	move.Args = cobra.ExactArgs(1)

	flag.Add(move,
		flag.Yes(),
		flag.Org(),
	)

	return move
}

// TODO: make internal once the move package is removed
func RunMove(ctx context.Context) error {
	appName := flag.FirstArg(ctx)

	client := client.FromContext(ctx).API()

	app, err := client.GetAppBasic(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed fetching app: %w", err)
	}

	logger := logger.FromContext(ctx)
	logger.Infof("app %q is currently in organization %q", app.Name, app.Organization.Slug)

	org, err := prompt.Org(ctx)
	if err != nil {
		return nil
	}

	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	if !flag.GetYes(ctx) {
		const msg = `Moving an app between organizations requires a complete shutdown and restart. This will result in some app downtime.
If the app relies on other services within the current organization, it may not come back up in a healthy manner.
Please confirm whether you wish to restart this app now.`
		fmt.Fprintln(io.ErrOut, colorize.Red(msg))

		switch confirmed, err := prompt.Confirmf(ctx, "Move app %s?", appName); {
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

	if _, err := client.MoveApp(ctx, appName, org.ID); err != nil {
		return fmt.Errorf("failed moving app: %w", err)
	}

	fmt.Fprintf(io.Out, "successfully moved %s to %s\n", appName, org.Slug)

	return nil
}
