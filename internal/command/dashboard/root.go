package dashboard

import (
	"context"
	"fmt"

	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func New() *cobra.Command {
	const (
		short = "Open web browser on Fly Web UI for this app"
		long  = `Open web browser on Fly Web UI for this application`
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
		short = "Open web browser on Fly Web UI for this app's metrics"
		long  = `Open web browser on Fly Web UI for this application's metrics`
	)
	cmd := command.New("metrics", short, long, runDashboardMetrics,
		command.RequireSession,
		command.RequireAppName,
	)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)
	return cmd
}

func runDashboard(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)
	return runDashboardOpen(ctx, "https://fly.io/apps/"+appName)
}

func runDashboardMetrics(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)
	return runDashboardOpen(ctx, "https://fly.io/apps/"+appName+"/metrics")
}

func runDashboardOpen(ctx context.Context, url string) error {
	io := iostreams.FromContext(ctx)
	fmt.Fprintln(io.Out, "Opening", url)
	return open.Run(url)
}
