package cmd

import (
	"fmt"
	"os"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppInfoCommand() *Command {
	return BuildCommand(runAppInfo, "info", "show detailed app information", os.Stdout, true, requireAppName)
}

func runAppInfo(ctx *CmdContext) error {
	app, err := ctx.FlyClient.GetApp(ctx.AppName())
	if err != nil {
		return err
	}

	fmt.Println(aurora.Bold("App"))
	err = ctx.RenderEx(&presenters.AppInfo{App: *app}, presenters.Options{HideHeader: true, Vertical: true})
	if err != nil {
		return err
	}

	fmt.Println(aurora.Bold("Services"))
	err = ctx.Render(&presenters.Services{Tasks: app.Tasks})
	if err != nil {
		return err
	}

	fmt.Println(aurora.Bold("IP Addresses"))
	err = ctx.Render(&presenters.IPAddresses{IPAddresses: app.IPAddresses.Nodes})
	if err != nil {
		return err
	}

	if app.Version == 0 {
		fmt.Println(`App has not been deployed yet. Try running "flyctl deploy nginxdemos/hello"`)
	}

	return nil
}
