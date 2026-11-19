package dashboard

import (
	"context"
	"fmt"

	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/iostreams"
)

func New() *cobra.Command {
	const (
		short = "Open the app's dashboard in a browser"
		long  = `Open the app's page in the Fly.io dashboard (https://fly.io/apps/<app>) in
your browser. The app comes from --app or fly.toml. Use
` + "`fly dashboard metrics`" + ` to open its metrics instead.`
	)
	cmd := command.New("dashboard", short, long, runDashboard,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.AddCommand(
		newDashboardMetrics(),
	)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)
	cmd.Aliases = []string{"dash"}

	return cmd
}

func newDashboardMetrics() *cobra.Command {
	const (
		short = "Open the app's metrics dashboard in a browser"
		long  = `Open the app's metrics page in the Fly.io dashboard in your browser. With
--grafana, open the app's dashboard on Fly.io's hosted Grafana
(fly-metrics.net) instead. The app comes from --app or fly.toml.`
	)
	cmd := command.New("metrics", short, long, runDashboardMetrics,
		command.RequireSession,
		command.RequireAppName,
	)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Bool{
			Name:        "grafana",
			Shorthand:   "g",
			Description: "Open Grafana metrics dashboard directly",
			Default:     false,
		},
	)

	return cmd
}

func runDashboard(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)

	return runDashboardOpen(ctx, "https://fly.io/apps/"+appName)
}

func runDashboardMetrics(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)

	if flag.GetBool(ctx, "grafana") {
		app, err := flapsutil.ClientFromContext(ctx).GetApp(ctx, appName)
		if err != nil {
			return fmt.Errorf("failed to get app info: %w", err)
		}

		url := fmt.Sprintf("https://fly-metrics.net/d/fly-app/fly-app?orgId=%d&var-app=%s", app.Organization.InternalNumericID, appName)

		return runDashboardOpen(ctx, url)
	}

	return runDashboardOpen(ctx, "https://fly.io/apps/"+appName+"/metrics")
}

func runDashboardOpen(ctx context.Context, url string) error {
	io := iostreams.FromContext(ctx)
	fmt.Fprintln(io.Out, "Opening", url)

	return open.Run(url)
}
