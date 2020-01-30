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

	if app.DeploymentStatus != nil {
		fmt.Println(aurora.Bold("Deployment Status"))
		err = ctx.RenderView(PresenterOption{
			Presentable: &presenters.DeploymentStatus{Status: app.DeploymentStatus},
			Vertical:    true,
		})

		if err != nil {
			return err
		}
	}

	fmt.Println(aurora.Bold("Allocations"))
	err = ctx.RenderView(PresenterOption{
		Presentable: &presenters.Allocations{Allocations: app.Allocations},
	})
	if err != nil {
		return err
	}

	return nil
}
