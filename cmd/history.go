package cmd

import (
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/client"

	"github.com/superfly/flyctl/docstrings"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newHistoryCommand(client *client.Client) *Command {
	historyStrings := docstrings.Get("history")
	return BuildCommand(nil, runHistory, historyStrings.Usage, historyStrings.Short, historyStrings.Long, client, requireSession, requireAppName)
}

func runHistory(commandContext *cmdctx.CmdContext) error {
	changes, err := commandContext.Client.API().GetAppChanges(commandContext.AppName)
	if err != nil {
		return err
	}

	return commandContext.Frender(cmdctx.PresenterOption{Presentable: &presenters.AppHistory{AppChanges: changes}})
}
