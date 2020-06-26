package cmd

import (
	"fmt"
	"github.com/superfly/flyctl/cmdctx"
	"os"

	"github.com/superfly/flyctl/docstrings"

	"github.com/skratchdot/open-golang/open"
)

func newDashboardCommand() *Command {
	ks := docstrings.Get("dashboard")
	ksCmd := BuildCommand(nil, runDashboard, ks.Usage, ks.Short, ks.Long, os.Stdout, requireSession, requireAppName)
	ksCmd.Aliases = []string{"dash"}

	km := docstrings.Get("dashboard.metrics")
	BuildCommand(ksCmd, runDashboardMetrics, km.Usage, km.Short, km.Long, os.Stdout, requireSession, requireAppName)
	return ksCmd
}

func runDashboard(ctx *cmdctx.CmdContext) error {
	return runDashboardOpen(ctx, false)
}

func runDashboardMetrics(ctx *cmdctx.CmdContext) error {
	return runDashboardOpen(ctx, true)
}

func runDashboardOpen(ctx *cmdctx.CmdContext, metrics bool) error {
	app, err := ctx.Client.API().GetApp(ctx.AppName)
	if err != nil {
		return err
	}

	docsURL := "https://fly.io/apps/" + app.Name
	if metrics {
		docsURL = docsURL + "/metrics"
	}

	fmt.Println("Opening", docsURL)
	return open.Run(docsURL)
}
