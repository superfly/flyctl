package cmd

import (
	"os"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppHistoryCommand() *Command {
	return BuildCommand(nil, runAppHistory, "history", "list app change history", os.Stdout, true, requireAppName)
}

func runAppHistory(ctx *CmdContext) error {
	changes, err := ctx.FlyClient.GetAppChanges(ctx.AppName())
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.AppHistory{AppChanges: changes})
}
