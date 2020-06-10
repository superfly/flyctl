package cmd

import (
	"fmt"
	"os"

	"github.com/segmentio/textio"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/docstrings"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppStatusCommand() *Command {
	statusStrings := docstrings.Get("status")
	cmd := BuildCommand(nil, runAppStatus, statusStrings.Usage, statusStrings.Short, statusStrings.Long, os.Stdout, requireSession, requireAppName)

	//TODO: Move flag descriptions to docstrings
	cmd.AddBoolFlag(BoolFlagOpts{Name: "all", Description: "Show completed allocations"})

	allocStatusStrings := docstrings.Get("status.alloc")
	allocStatusCmd := BuildCommand(cmd, runAllocStatus, allocStatusStrings.Usage, allocStatusStrings.Short, allocStatusStrings.Long, os.Stdout, requireSession, requireAppName)
	allocStatusCmd.Args = cobra.ExactArgs(1)
	return cmd
}

func runAppStatus(ctx *CmdContext) error {
	app, err := ctx.Client.API().GetAppStatus(ctx.AppName, ctx.Config.GetBool("all"))
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
		err = ctx.Frender(ctx.Out, PresenterOption{
			Presentable: &presenters.DeploymentStatus{Status: app.DeploymentStatus},
			Vertical:    true,
		})

		if err != nil {
			return err
		}
	}

	fmt.Println(aurora.Bold("Allocations"))
	err = ctx.Frender(ctx.Out, PresenterOption{
		Presentable: &presenters.Allocations{Allocations: app.Allocations},
	})
	if err != nil {
		return err
	}

	return nil
}

func runAllocStatus(ctx *CmdContext) error {
	alloc, err := ctx.Client.API().GetAllocationStatus(ctx.AppName, ctx.Args[0], 25)
	if err != nil {
		return err
	}

	if alloc == nil {
		return api.ErrNotFound
	}

	err = ctx.Frender(
		ctx.Out,
		PresenterOption{
			Title: "Allocation",
			Presentable: &presenters.Allocations{
				Allocations: []*api.AllocationStatus{alloc},
			},
			Vertical: true,
		},
		PresenterOption{
			Title: "Recent Events",
			Presentable: &presenters.AllocationEvents{
				Events: alloc.Events,
			},
		},
		PresenterOption{
			Title: "Checks",
			Presentable: &presenters.AllocationChecks{
				Checks: alloc.Checks,
			},
		},
	)
	if err != nil {
		return err
	}

	fmt.Println(aurora.Bold("Recent Logs"))
	p := textio.NewPrefixWriter(ctx.Out, "  ")
	logPresenter := presenters.LogPresenter{HideAllocID: true, HideRegion: true, RemoveNewlines: true}
	logPresenter.FPrint(p, alloc.RecentLogs)
	p.Flush()

	return nil
}
