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
	tasks, err := ctx.FlyClient.GetAppTasks(ctx.AppName())
	if err != nil {
		return err
	}

	fmt.Println(aurora.Bold("Tasks"))
	err = ctx.RenderEx(&presenters.TaskSummary{Tasks: tasks}, presenters.Options{HideHeader: true})
	if err != nil {
		return err
	}

	fmt.Println(aurora.Bold("Allocations"))
	err = ctx.Render(&presenters.Allocations{Tasks: tasks})
	if err != nil {
		return err
	}

	return nil
}
