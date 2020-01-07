package cmd

import (
	"fmt"
	"github.com/superfly/flyctl/docstrings"
	"os"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppStatusCommand() *Command {
	statusStrings := docstrings.Get("status")
	cmd := BuildCommand(nil, runAppStatus, statusStrings.Usage, statusStrings.Short, statusStrings.Long, true, os.Stdout, requireAppName)

	//TODO: Move flag descriptions to docstrings
	cmd.AddBoolFlag(BoolFlagOpts{Name: "all", Description: "Show completed allocations"})

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
		fmt.Println(`App has not been deployed yet.`)
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
