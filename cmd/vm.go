package cmd

import (
	"fmt"
	"io"

	"github.com/logrusorgru/aurora"
	"github.com/segmentio/textio"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"
)

func newVMCommand(client *client.Client) *Command {
	vmCmd := BuildCommandKS(nil, nil, docstrings.Get("vm"), client)

	vmRestartCmd := BuildCommandKS(vmCmd, runVMRestart, docstrings.Get("vm.restart"), client, requireSession, requireAppName)
	vmRestartCmd.Args = cobra.ExactArgs(1)

	vmStopCmd := BuildCommandKS(vmCmd, runVMStop, docstrings.Get("vm.stop"), client, requireSession, requireAppName)
	vmStopCmd.Args = cobra.ExactArgs(1)

	vmStatusCmd := BuildCommandKS(vmCmd, runVMStatus, docstrings.Get("vm.status"), client, requireSession, requireAppName)
	vmStatusCmd.Args = cobra.ExactArgs(1)

	return vmCmd
}

func runVMRestart(cmdctx *cmdctx.CmdContext) error {
	ctx := cmdctx.Command.Context()

	appName := cmdctx.AppName
	allocID := cmdctx.Args[0]

	err := cmdctx.Client.API().RestartAllocation(ctx, appName, allocID)
	if err != nil {
		return err
	}

	fmt.Printf("VM %s is being restarted\n", allocID)
	return nil
}

func runVMStop(cmdctx *cmdctx.CmdContext) error {
	ctx := cmdctx.Command.Context()

	appName := cmdctx.AppName
	allocID := cmdctx.Args[0]

	err := cmdctx.Client.API().StopAllocation(ctx, appName, allocID)
	if err != nil {
		return err
	}

	fmt.Printf("VM %s is being stopped\n", allocID)
	return nil
}

func runVMStatus(cmdCtx *cmdctx.CmdContext) error {
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
