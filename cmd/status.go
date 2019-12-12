package cmd

import (
	"fmt"
	"os"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppStatusCommand() *Command {
	cmd := BuildCommand(nil, runAppStatus, "status", "show app status", os.Stdout, true, requireAppName)
	cmd.AddBoolFlag(BoolFlagOpts{Name: "all", Description: "show completed allocations"})

	return cmd
}

func runAppStatus(ctx *CmdContext) error {
	app, err := ctx.FlyClient.GetAppStatus(ctx.AppName, ctx.Config.GetBool("all"))
	if err != nil {
		return err
	}

	fmt.Println(aurora.Bold("App"))
	err = ctx.RenderEx(&presenters.AppInfo{App: *app}, presenters.Options{HideHeader: true, Vertical: true})
	if err != nil {
		return err
	}

	if !app.Deployed {
		fmt.Println(`App has not been deployed yet. Try running "flyctl deploy --image nginxdemos/hello"`)
		return nil
	}

	fmt.Println(aurora.Bold("Tasks"))
	err = ctx.RenderEx(&presenters.TaskSummary{Tasks: app.Tasks}, presenters.Options{HideHeader: true})
	if err != nil {
		return err
	}

	fmt.Println(aurora.Bold("Latest Deployment"))
	err = ctx.RenderEx(&presenters.ReleaseDetails{Release: *app.CurrentRelease}, presenters.Options{Vertical: true})
	if err != nil {
		return err
	}

	fmt.Println(aurora.Bold("Deployment Status"))
	err = ctx.Render(&presenters.DeploymentTaskStatus{Release: *app.CurrentRelease})
	if err != nil {
		return err
	}

	fmt.Println(aurora.Bold("Allocations"))
	err = ctx.Render(&presenters.Allocations{Tasks: app.Tasks})
	if err != nil {
		return err
	}

	return nil
}
