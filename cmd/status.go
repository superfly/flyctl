package cmd

import (
	"fmt"
	"os"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppStatusCommand() *Command {
	cmd := BuildCommand(nil, runAppStatus, "status", "show app status", os.Stdout, true, requireAppName)

	return cmd
}

func runAppStatus(ctx *CmdContext) error {
	app, err := ctx.FlyClient.GetAppWithTasks(ctx.AppName())
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

	fmt.Println(aurora.Bold("Allocations"))
	err = ctx.Render(&presenters.Allocations{Tasks: app.Tasks})
	if err != nil {
		return err
	}

	return nil
}
