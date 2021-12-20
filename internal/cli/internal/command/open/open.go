package open

import (
	"context"
	"errors"
	"fmt"

	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/client"
)

func New() *cobra.Command {
	const (
		long = `Open browser to current deployed application. If an optional path is specified, this is appended to the
		URL for deployed application
`
		short = "Open browser to current deployed application"

		usage = "open [PATH]"
	)

	cmd := command.New(usage, short, long, run, command.RequireSession, command.RequireAppName)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func run(ctx context.Context) error {
	var (
		path    = flag.FirstArg(ctx)
		appName = app.NameFromContext(ctx)
	)

	app, err := client.FromContext(ctx).API().GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app: %w", err)
	}

	if !app.Deployed {
		return errors.New("app has not been deployed yet. Please try deploying your app first")
	}

	appUrl := "http://" + app.Hostname + path

	iostream := iostreams.FromContext(ctx)

	fmt.Fprintf(iostream.Out, "Opening %s ...\n", appUrl)

	if err := open.Run(appUrl); err != nil {
		return fmt.Errorf("failed opening URL %s: %w", appUrl, err)
	}

	return nil
}
