package apps

import (
	"context"
	"errors"
	"fmt"

	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

// TODO: make internal once the open command has been deprecated
func NewOpen() (cmd *cobra.Command) {
	const (
		long = `Open browser to current deployed application. If an optional relative URI is specified, it is appended
to the root URL of the deployed application.
`
		short = "Open browser to current deployed application"

		usage = "open [RELATIVE_URI]"
	)

	cmd = command.New(usage, short, long, runOpen,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return
}

func runOpen(ctx context.Context) error {
	iostream := iostreams.FromContext(ctx)
	appName := appconfig.NameFromContext(ctx)

	app, err := client.FromContext(ctx).API().GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	if !app.Deployed && app.PlatformVersion != "machines" {
		return errors.New("app has not been deployed yet. Please try deploying your app first")
	}

	appConfig := appconfig.ConfigFromContext(ctx)
	if appConfig == nil {
		if appConfig, err = appconfig.FromAppCompact(ctx, app); err != nil {
			return errors.New("The app config could not be found")
		}
	}

	appURL := appConfig.URL()
	if appURL == nil {
		return errors.New("The app doesn't expose a public http service")
	}

	if relURI := flag.FirstArg(ctx); relURI != "" {
		newURL, err := appURL.Parse(relURI)
		if err != nil {
			return fmt.Errorf("failed to parse relative URI '%s': %w", relURI, err)
		}
		appURL = newURL
	}

	fmt.Fprintf(iostream.Out, "opening %s ...\n", appURL)
	if err := open.Run(appURL.String()); err != nil {
		return fmt.Errorf("failed opening %s: %w", appURL, err)
	}

	return nil
}
