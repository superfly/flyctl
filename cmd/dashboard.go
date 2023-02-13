package cmd

import (
	"fmt"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/cmdctx"

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
	return runDashboardOpen(cmdCtx, "https://fly.io/apps/"+cmdCtx.AppName)
}

func runDashboardMetrics(cmdCtx *cmdctx.CmdContext) error {
	return runDashboardOpen(cmdCtx, "https://fly.io/apps/"+cmdCtx.AppName+"/metrics")
}

func runDashboardOpen(ctx *cmdctx.CmdContext, url string) error {
	fmt.Println("Opening", url)
	return open.Run(url)
}
