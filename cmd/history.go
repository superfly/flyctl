package cmd

import (
	"github.com/superfly/flyctl/docstrings"
	"os"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppHistoryCommand() *Command {
	historyStrings := docstrings.Get("history")
	return BuildCommand(nil, runAppHistory, historyStrings.Usage, historyStrings.Short, historyStrings.Long, true, os.Stdout, requireAppName)
}

func runAppHistory(ctx *CmdContext) error {
	changes, err := ctx.FlyClient.GetAppChanges(ctx.AppName)
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.AppHistory{AppChanges: changes})
}
