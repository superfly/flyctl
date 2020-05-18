package cmd

import (
	"fmt"
	"os"

	"github.com/superfly/flyctl/docstrings"

	"github.com/skratchdot/open-golang/open"
)

func newOpenCommand() *Command {
	ks := docstrings.Get("open")
	return BuildCommand(nil, runOpen, ks.Usage, ks.Short, ks.Long, os.Stdout, requireSession, requireAppName)
}

func runOpen(ctx *CmdContext) error {
	app, err := ctx.Client.API().GetApp(ctx.AppName)
	if err != nil {
		return err
	}

	if !app.Deployed {
		fmt.Println(`App has not been deployed yet. Try running "flyctl deploy --image flyio/hellofly"`)
		return nil
	}

	docsURL := "http://" + app.Hostname + "/"
	fmt.Println("Opening", docsURL)
	return open.Run(docsURL)
}
