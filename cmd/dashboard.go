package cmd

import (
	"fmt"

	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/client"

	"github.com/superfly/flyctl/docstrings"

	"github.com/skratchdot/open-golang/open"
)

func newDashboardCommand(client *client.Client) *Command {
	dashboardStrings := docstrings.Get("dashboard")
	dashboardCmd := BuildCommandKS(nil, runDashboard, dashboardStrings, client, nil, requireSession, requireAppName)
	dashboardCmd.Aliases = []string{"dash"}

	dashMetricsStrings := docstrings.Get("dashboard.metrics")
	BuildCommandKS(dashboardCmd, runDashboardMetrics, dashMetricsStrings, client, nil, requireSession, requireAppName)

	return dashboardCmd
}

func runDashboard(ctx *cmdctx.CmdContext) error {
	app, err := ctx.Client.API().GetApp(ctx.AppName)
	if err != nil {
		return err
	}

	dashURL := "https://fly.io/apps/" + app.Name
	return runDashboardOpen(ctx, dashURL)
}

func runDashboardMetrics(ctx *cmdctx.CmdContext) error {
	app, err := ctx.Client.API().GetApp(ctx.AppName)
	if err != nil {
		return err
	}

	metricsURL := "https://fly.io/apps/" + app.Name + "/metrics"

	return runDashboardOpen(ctx, metricsURL)
}

func runDashboardOpen(ctx *cmdctx.CmdContext, url string) error {
	fmt.Println("Opening", url)
	return open.Run(url)
}
