package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/superfly/flyctl/cmdctx"

	"github.com/segmentio/textio"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/docstrings"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/cmd/presenters"
)

func newStatusCommand() *Command {
	statusStrings := docstrings.Get("status")
	cmd := BuildCommandKS(nil, runStatus, statusStrings, os.Stdout, requireSession, requireAppNameAsArg)

	//TODO: Move flag descriptions to docstrings
	cmd.AddBoolFlag(BoolFlagOpts{Name: "all", Description: "Show completed allocations"})

	allocStatusStrings := docstrings.Get("status.alloc")
	allocStatusCmd := BuildCommand(cmd, runAllocStatus, allocStatusStrings.Usage, allocStatusStrings.Short, allocStatusStrings.Long, os.Stdout, requireSession, requireAppName)
	allocStatusCmd.Args = cobra.ExactArgs(1)
	return cmd
}

func runStatus(ctx *cmdctx.CmdContext) error {
	app, err := ctx.Client.API().GetAppStatus(ctx.AppName, ctx.Config.GetBool("all"))

	if err != nil {
		return err
	}

	_, backupregions, err := ctx.Client.API().ListAppRegions(ctx.AppName)

	err = ctx.Frender(cmdctx.PresenterOption{Presentable: &presenters.AppStatus{AppStatus: *app}, HideHeader: true, Vertical: true, Title: "App"})
	if err != nil {
		return err
	}

	// If JSON output, everything has been printed, so return
	if ctx.OutputJSON() {
		return nil
	}

	// Continue formatted output
	if !app.Deployed {
		fmt.Println(`App has not been deployed yet.`)
		return nil
	}

	if app.DeploymentStatus != nil {
		err = ctx.Frender(cmdctx.PresenterOption{
			Presentable: &presenters.DeploymentStatus{Status: app.DeploymentStatus},
			Vertical:    true,
			Title:       "Deployment Status",
		})

		if err != nil {
			return err
		}
	}

	err = ctx.Frender(cmdctx.PresenterOption{
		Presentable: &presenters.Allocations{Allocations: app.Allocations, BackupRegions: backupregions},
		Title:       "Allocations",
	})

	if err != nil {
		return err
	}

	return nil
}

func runAllocStatus(ctx *cmdctx.CmdContext) error {
	alloc, err := ctx.Client.API().GetAllocationStatus(ctx.AppName, ctx.Args[0], 25)
	if err != nil {
		return err
	}

	if alloc == nil {
		return api.ErrNotFound
	}

	err = ctx.Frender(
		cmdctx.PresenterOption{
			Title: "Allocation",
			Presentable: &presenters.Allocations{
				Allocations: []*api.AllocationStatus{alloc},
			},
			Vertical: true,
		},
		cmdctx.PresenterOption{
			Title: "Recent Events",
			Presentable: &presenters.AllocationEvents{
				Events: alloc.Events,
			},
		},
		cmdctx.PresenterOption{
			Title: "Checks",
			Presentable: &presenters.AllocationChecks{
				Checks: alloc.Checks,
			},
		},
	)
	if err != nil {
		return err
	}

	var p io.Writer
	var pw *textio.PrefixWriter

	if !ctx.OutputJSON() {
		fmt.Println(aurora.Bold("Recent Logs"))
		pw = textio.NewPrefixWriter(ctx.Out, "  ")
		p = pw
	} else {
		p = ctx.Out
	}

	logPresenter := presenters.LogPresenter{HideAllocID: true, HideRegion: true, RemoveNewlines: true}
	logPresenter.FPrint(p, ctx.OutputJSON(), alloc.RecentLogs)

	if p != ctx.Out {
		pw.Flush()
	}

	return nil
}
