package cmd

import (
	"os"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppListCommand() *Command {
	return BuildCommand(nil, runAppsList, "apps", "list apps", os.Stdout, true)
}

func runAppsList(ctx *CmdContext) error {
	apps, err := ctx.FlyClient.GetApps()
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.Apps{Apps: apps})
}
