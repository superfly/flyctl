package cmd

import (
	"os"

	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/flyname"

	"github.com/superfly/flyctl/docstrings"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newInfoCommand() *Command {
	ks := docstrings.Get("info")
	appInfoCmd := BuildCommandKS(nil, runInfo, ks, os.Stdout, requireSession, requireAppName)

	appInfoCmd.AddBoolFlag(BoolFlagOpts{
		Name:        "name",
		Shorthand:   "n",
		Description: "Returns just the appname",
	})

	appInfoCmd.AddBoolFlag(BoolFlagOpts{
		Name:        "host",
		Description: "Returns just the hostname",
	})
	return appInfoCmd
}

func runInfo(ctx *cmdctx.CmdContext) error {
	app, err := ctx.Client.API().GetAppCompact(ctx.AppName)
	if err != nil {
		return err
	}

	if ctx.Config.GetBool("name") {
		ctx.Status("info", cmdctx.SINFO, app.Name)
		return nil
	}

	if ctx.Config.GetBool("host") {
		ctx.Status("info", cmdctx.SINFO, app.Hostname)
		return nil
	}

	err = ctx.Frender(cmdctx.PresenterOption{Presentable: &presenters.AppCompact{AppCompact: *app}, HideHeader: true, Vertical: true, Title: "App"})
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
			ctx.Status("info", `App has not been deployed yet. Try running "`+flyname.Name()+` deploy --image flyio/hellofly"`)
		}
	}
	return nil
}
