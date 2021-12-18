package open

import (
	"context"
	"fmt"

	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/buildinfo"
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

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func run(ctx context.Context) (err error) {
	var (
		path = "/"

		appName = app.NameFromContext(ctx)
	)

	if flag.Len(ctx) > 1 {
		return fmt.Errorf("too many arguments - only one path argument allowed")
	}

	if flag.Len(ctx) > 0 {
		path = flag.FirstArg(ctx)
	}

	app, err := client.FromContext(ctx).API().GetApp(ctx, appName)
	if err != nil {
		return
	}

	if !app.Deployed {
		fmt.Println(`App has not been deployed yet. Try running "` + buildinfo.Name() + ` deploy --image flyio/hellofly"`)
		return nil
	}

	appUrl := "http://" + app.Hostname + path
	fmt.Println("Opening", appUrl)

	return open.Run(appUrl)

}
