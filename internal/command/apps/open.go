package apps

import (
	"context"
	"errors"
	"fmt"

	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/iostreams"
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

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return fmt.Errorf("could not create flaps client: %w", err)
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	appConfig := appconfig.ConfigFromContext(ctx)
	if appConfig == nil {
		appConfig, err = appconfig.FromRemoteApp(ctx, appName)
		if err != nil {
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
