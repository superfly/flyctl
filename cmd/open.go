package cmd

import (
	"fmt"

	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/client"

	"github.com/superfly/flyctl/docstrings"

	"github.com/skratchdot/open-golang/open"
)

func newOpenCommand(client *client.Client) *Command {
	ks := docstrings.Get("open")
	opencommand := BuildCommandKS(nil, runOpen, ks, client, requireSession, requireAppName)
	return opencommand
}

func runOpen(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	var path = "/"

	if len(cmdCtx.Args) > 1 {
		return fmt.Errorf("too many arguments - only one path argument allowed")
	}

	if len(cmdCtx.Args) > 0 {
		path = cmdCtx.Args[0]
	}

	app, err := cmdCtx.Client.API().GetApp(ctx, cmdCtx.AppName)
	if err != nil {
		return err
	}

	if !app.Deployed {
		fmt.Println(`App has not been deployed yet. Try running "` + buildinfo.Name() + ` deploy --image flyio/hellofly"`)
		return nil
	}

	if len(cmdCtx.Args) > 1 {
		return fmt.Errorf("too many arguments - only one path argument allowed")
	}

	if len(cmdCtx.Args) > 0 {
		path = cmdCtx.Args[0]
	}

	docsURL := "http://" + app.Hostname + path
	fmt.Println("Opening", docsURL)
	return open.Run(docsURL)
}
