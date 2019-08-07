package cmd

import (
	"os"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppInfoCommand() *Command {
	return BuildCommand(runAppInfo, "info", "show app info", os.Stdout, true, requireAppName)
}

func runAppInfo(ctx *CmdContext) error {
	app, err := ctx.FlyClient.GetApp(ctx.AppName())
	if err != nil {
		return err
	}

	return ctx.RenderEx(&presenters.AppInfo{App: *app}, presenters.Options{HideHeader: true, Vertical: true})
}
