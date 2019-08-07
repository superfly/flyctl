package cmd

import (
	"fmt"
	"os"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppStatusCommand() *Command {
	cmd := BuildCommand(runAppStatus, "status", "show app status", os.Stdout, true, requireAppName)

	return cmd
}

func runAppStatus(ctx *CmdContext) error {
	services, err := ctx.FlyClient.GetAppServices(ctx.AppName())
	if err != nil {
		return err
	}

	fmt.Println(aurora.Bold("Services"))
	err = ctx.RenderEx(&presenters.Services{Services: services}, presenters.Options{HideHeader: true})
	if err != nil {
		return err
	}

	fmt.Println(aurora.Bold("Allocations"))
	err = ctx.Render(&presenters.Allocations{Services: services})
	if err != nil {
		return err
	}

	return nil
}
