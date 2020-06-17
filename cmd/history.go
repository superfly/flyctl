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

func runAppHistory(commandContext *cmdctx.CmdContext) error {
	changes, err := commandContext.Client.API().GetAppChanges(commandContext.AppName)
	if err != nil {
		return err
	}

	return commandContext.Frender(cmdctx.PresenterOption{Presentable: &presenters.AppHistory{AppChanges: changes}})
}
