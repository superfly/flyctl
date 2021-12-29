package cmd

import (
	"fmt"
	"io"

	"github.com/superfly/flyctl/cmdctx"

	"github.com/segmentio/textio"
	"github.com/superfly/flyctl/api"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/cmd/presenters"
)

func runAllocStatus(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	alloc, err := cmdCtx.Client.API().GetAllocationStatus(ctx, cmdCtx.AppName, cmdCtx.Args[0], 25)
	if err != nil {
		return err
	}

	if alloc == nil {
		return api.ErrNotFound
	}

	err = cmdCtx.Frender(
		cmdctx.PresenterOption{
			Title: "Instance",
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

	if !cmdCtx.OutputJSON() {
		fmt.Println(aurora.Bold("Recent Logs"))
		pw = textio.NewPrefixWriter(cmdCtx.Out, "  ")
		p = pw
	} else {
		p = cmdCtx.Out
	}

	// logPresenter := presenters.LogPresenter{HideAllocID: true, HideRegion: true, RemoveNewlines: true}
	// logPresenter.FPrint(p, ctx.OutputJSON(), alloc.RecentLogs)

	if p != cmdCtx.Out {
		_ = pw.Flush()
	}

	return nil
}
