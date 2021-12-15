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
	dashboardCmd := BuildCommandKS(nil, runDashboard, dashboardStrings, client, requireSession, requireAppName)
	dashboardCmd.Aliases = []string{"dash"}

	dashMetricsStrings := docstrings.Get("dashboard.metrics")
	BuildCommandKS(dashboardCmd, runDashboardMetrics, dashMetricsStrings, client, requireSession, requireAppName)

	return dashboardCmd
}

func runDashboard(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	app, err := cmdCtx.Client.API().GetApp(ctx, cmdCtx.AppName)
	if err != nil {
		return err
	}

	dashURL := "https://fly.io/apps/" + app.Name
	return runDashboardOpen(cmdCtx, dashURL)
}

func runDashboardMetrics(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	app, err := cmdCtx.Client.API().GetApp(ctx, cmdCtx.AppName)
	if err != nil {
		return err
	}

	metricsURL := "https://fly.io/apps/" + app.Name + "/metrics"

	return runDashboardOpen(cmdCtx, metricsURL)
}

func runDashboardOpen(ctx *cmdctx.CmdContext, url string) error {
	fmt.Println("Opening", url)
	return open.Run(url)
}
