package cmd

import (
	"github.com/superfly/flyctl/cmdctx"
	"os"

	"github.com/superfly/flyctl/docstrings"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppHistoryCommand() *Command {
	historyStrings := docstrings.Get("history")
	return BuildCommand(nil, runAppHistory, historyStrings.Usage, historyStrings.Short, historyStrings.Long, os.Stdout, requireSession, requireAppName)
}

func runAppHistory(ctx *cmdctx.CmdContext) error {
	changes, err := ctx.Client.API().GetAppChanges(ctx.AppName)
	if err != nil {
		return err
	}

	return ctx.Frender(ctx.Out, cmdctx.PresenterOption{Presentable: &presenters.AppHistory{AppChanges: changes}})
}
