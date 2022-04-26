package monitor

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/watch"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/flag"
)

// New initializes and returns a new monitor Command.
func New() (cmd *cobra.Command) {
	const (
		short = `Monitor currently running application deployments`
		long  = short + `. Use Control-C to stop output.`
	)

	cmd = command.New("monitor", short, long, run,
		command.RequireSession,
		command.RequireAppName)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return
}

func run(ctx context.Context) (err error) {

	appName := app.NameFromContext(ctx)
	client := client.FromContext(ctx).API()

	app, err := client.GetApp(ctx, appName)

	if err != nil {
		return fmt.Errorf("failed to get app from context")
	}

	if app.CurrentRelease == nil {
		return fmt.Errorf("app %s has not been deployed yet", appName)
	}
	if !app.CurrentRelease.InProgress {
		return fmt.Errorf("app %s is not currently deploying a release. The build and release command must succeed before a release is deployed. The latest release version is %d", appName, app.CurrentRelease.Version)
	}

	return watch.Deployment(ctx, app.CurrentRelease.EvaluationID)

}
