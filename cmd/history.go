package cmd

import (
	"os"

	"github.com/superfly/flyctl/docstrings"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppHistoryCommand() *Command {
	historyStrings := docstrings.Get("history")
	return BuildCommand(nil, runAppHistory, historyStrings.Usage, historyStrings.Short, historyStrings.Long, os.Stdout, requireSession, requireAppName)
}

func runAppHistory(ctx *CmdContext) error {
	changes, err := ctx.Client.API().GetAppChanges(ctx.AppName)
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.AppHistory{AppChanges: changes})
}
