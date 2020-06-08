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

	var path = "/"

	if len(ctx.Args) > 1 {
		return fmt.Errorf("too many arguments - only one path argument allowed")
	}

	if len(ctx.Args) > 0 {
		path = ctx.Args[0]
	}

	docsURL := "http://" + app.Hostname + path
	fmt.Println("Opening", docsURL)
	return open.Run(docsURL)
}
