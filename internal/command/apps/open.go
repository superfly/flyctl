package apps

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/logger"
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

var (
	openBrowser         = open.Run
	loadRemoteAppConfig = appconfig.FromRemoteApp
)

func runOpen(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)
	if appName == "" {
		return command.ErrRequireAppName
	}

	appConfig := appconfig.ConfigFromContext(ctx)
	if appConfig != nil && appConfig.AppName != appName {
		appConfig = nil
	}
	if appConfig == nil {
		var err error
		appConfig, err = loadRemoteAppConfig(ctx, appName)
		if err != nil || appConfig == nil {
			if log := logger.MaybeFromContext(ctx); log != nil && err != nil {
				log.Debugf("failed loading remote app config for %s: %v", appName, err)
			}

			return openAppURL(ctx, defaultAppURL(appName))
		}
	}

	appURL := appConfig.URL()
	if appURL == nil {
		return errors.New("The app doesn't expose a public http service")
	}

	return openAppURL(ctx, appURL)
}

func defaultAppURL(appName string) *url.URL {
	return &url.URL{
		Scheme: "https",
		Host:   appName + ".fly.dev",
		Path:   "/",
	}
}

func openAppURL(ctx context.Context, appURL *url.URL) error {
	iostream := iostreams.FromContext(ctx)

	if relURI := flag.FirstArg(ctx); relURI != "" {
		newURL, err := appURL.Parse(relURI)
		if err != nil {
			return fmt.Errorf("failed to parse relative URI '%s': %w", relURI, err)
		}
		appURL = newURL
	}

	fmt.Fprintf(iostream.Out, "opening %s ...\n", appURL)
	if err := openBrowser(appURL.String()); err != nil {
		return fmt.Errorf("failed opening %s: %w", appURL, err)
	}

	return nil
}
