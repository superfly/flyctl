package cmd

import (
	"github.com/superfly/flyctl/cmdctx"
	"os"

	"github.com/superfly/flyctl/docstrings"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppInfoCommand() *Command {
	ks := docstrings.Get("info")
	return BuildCommand(nil, runAppInfo, ks.Usage, ks.Short, ks.Long, os.Stdout, requireSession, requireAppName)
}

func runAppInfo(ctx *cmdctx.CmdContext) error {
	app, err := ctx.Client.API().GetApp(ctx.AppName)
	if err != nil {
		return err
	}

	err = ctx.Frender(cmdctx.PresenterOption{Presentable: &presenters.AppInfo{App: *app}, HideHeader: true, Vertical: true, Title: "App"})
	if err != nil {
		return err
	}

	// For JSON, everything is included in the previous render, for humans, we need to do some formatting
	if !ctx.OutputJSON() {
		err = ctx.Frender(cmdctx.PresenterOption{Presentable: &presenters.Services{Services: app.Services}, Title: "Services"})
		if err != nil {
			return err
		}

		err = ctx.Frender(cmdctx.PresenterOption{Presentable: &presenters.IPAddresses{IPAddresses: app.IPAddresses.Nodes}, Title: "IP Adresses"})
		if err != nil {
			return err
		}

		if !app.Deployed {
			ctx.Status("flyctl", `App has not been deployed yet. Try running "flyctl deploy --image flyio/hellofly"`)
		}
	}
	return nil
}
