package cmd

import (
	"fmt"
	"os"

	"github.com/superfly/flyctl/docstrings"

	"github.com/logrusorgru/aurora"
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

	fmt.Println(aurora.Bold("App"))
	err = ctx.RenderEx(&presenters.AppInfo{App: *app}, presenters.Options{HideHeader: true, Vertical: true})
	if err != nil {
		return err
	}

	fmt.Println(aurora.Bold("Services"))
	err = ctx.Render(&presenters.Services{Services: app.Services})
	if err != nil {
		return err
	}

	fmt.Println(aurora.Bold("IP Addresses"))
	err = ctx.Render(&presenters.IPAddresses{IPAddresses: app.IPAddresses.Nodes})
	if err != nil {
		return err
	}

	if !app.Deployed {
		fmt.Println(`App has not been deployed yet. Try running "flyctl deploy --image nginxdemos/hello"`)
	}

	return nil
}
