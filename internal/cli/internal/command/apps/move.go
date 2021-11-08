package apps

import (
	"context"
	"fmt"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/logger"
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
		command.RequireSession)

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

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed fetching app: %w", err)
	}

	logger := logger.FromContext(ctx)
	logger.Infof("app %q is currently in organization %q", app.Name, app.Organization.Slug)

	org, err := prompt.Org(ctx, nil)
	if err != nil {
		return nil
	}

	io := iostreams.FromContext(ctx)

	if !flag.GetYes(ctx) {
		fmt.Fprintln(io.ErrOut, aurora.Red(`Moving an app between organizations requires a complete shutdown and restart. This will result in some app downtime.
If the app relies on other services within the current organization, it may not come back up in a healthy manner.
Please confirm you wish to restart this app now?`))

		msg := fmt.Sprintf("Destroy app %s?", appName)
		if confirmed, err := prompt.Confirm(ctx, msg); err != nil || !confirmed {
			return err
		}
	}

	if _, err := client.MoveApp(ctx, appName, org.ID); err != nil {
		return fmt.Errorf("failed moving app: %w", err)
	}

	fmt.Fprintf(io.Out, "successfully moved %s to %s\n", appName, org.Slug)

	return nil
}
