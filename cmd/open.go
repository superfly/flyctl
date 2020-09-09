package cmd

import (
	"fmt"
	"os"

	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/flyname"

	"github.com/superfly/flyctl/docstrings"

	"github.com/skratchdot/open-golang/open"
)

func newOpenCommand() *Command {
	ks := docstrings.Get("open")
	opencommand := BuildCommandKS(nil, runOpen, ks, os.Stdout, requireSession, requireAppName)
	return opencommand
}

func runOpen(ctx *cmdctx.CmdContext) error {
	var path = "/"

	if len(ctx.Args) > 1 {
		return fmt.Errorf("too many arguments - only one path argument allowed")
	}

	if len(ctx.Args) > 0 {
		path = ctx.Args[0]
	}

	app, err := ctx.Client.API().GetApp(ctx.AppName)
	if err != nil {
		return err
	}

	if !app.Deployed {
		fmt.Println(`App has not been deployed yet. Try running "` + flyname.Name() + ` deploy --image flyio/hellofly"`)
		return nil
	}

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
