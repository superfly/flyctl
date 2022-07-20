package cmd

import (
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/buildinfo"

	"github.com/superfly/flyctl/docstrings"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newInfoCommand(client *client.Client) *Command {
	ks := docstrings.Get("info")
	appInfoCmd := BuildCommandKS(nil, runInfo, ks, client, requireSession, requireAppName)

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

func runInfo(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	app, err := cmdCtx.Client.API().GetAppInfo(ctx, cmdCtx.AppName)
	if err != nil {
		return err
	}

	if cmdCtx.Config.GetBool("name") {
		cmdCtx.Status("info", cmdctx.SINFO, app.Name)
		return nil
	}

	if cmdCtx.Config.GetBool("host") {
		cmdCtx.Status("info", cmdctx.SINFO, app.Hostname)
		return nil
	}

	err = cmdCtx.Frender(cmdctx.PresenterOption{Presentable: &presenters.AppInfo{AppInfo: *app}, HideHeader: true, Vertical: true, Title: "App"})
	if err != nil {
		return err
	}

	// For JSON, everything is included in the previous render, for humans, we need to do some formatting
	if !cmdCtx.OutputJSON() {
		err = cmdCtx.Frender(cmdctx.PresenterOption{Presentable: &presenters.Services{Services: app.Services}, Title: "Services"})
		if err != nil {
			return err
		}

		err = cmdCtx.Frender(cmdctx.PresenterOption{Presentable: &presenters.IPAddresses{IPAddresses: app.IPAddresses.Nodes}, Title: "IP Adresses"})
		if err != nil {
			return err
		}

		if !app.Deployed {
			cmdCtx.Status("info", `App has not been deployed yet. Try running "`+buildinfo.Name()+` deploy --image flyio/hellofly"`)
		}
	}
	return nil
}
