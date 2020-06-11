package cmd

import (
	"fmt"
	"github.com/superfly/flyctl/flyctl"
	"os"

	"github.com/superfly/flyctl/docstrings"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppInfoCommand() *Command {
	ks := docstrings.Get("info")
	return BuildCommand(nil, runAppInfo, ks.Usage, ks.Short, ks.Long, os.Stdout, requireSession, requireAppName)
}

func runAppInfo(ctx *CmdContext) error {
	app, err := ctx.Client.API().GetApp(ctx.AppName)
	if err != nil {
		return err
	}

	err = ctx.Frender(ctx.Out, PresenterOption{Presentable: &presenters.AppInfo{App: *app}, HideHeader: true, Vertical: true, Title: "App"})
	if err != nil {
		return err
	}

	// For JSON, everything is included in the previous render, for humans, we need to do some formatting
	if !ctx.GlobalConfig.GetBool(flyctl.ConfigJSONOutput) {
		err = ctx.Frender(ctx.Out, PresenterOption{Presentable: &presenters.Services{Services: app.Services}, Title: "Services"})
		if err != nil {
			return err
		}

		err = ctx.Frender(ctx.Out, PresenterOption{Presentable: &presenters.IPAddresses{IPAddresses: app.IPAddresses.Nodes}, Title: "IP Adresses"})
		if err != nil {
			return err
		}

		if !app.Deployed {
			fmt.Fprintln(ctx.Out, `App has not been deployed yet. Try running "flyctl deploy --image flyio/hellofly"`)
		}
	}
	return nil
}
