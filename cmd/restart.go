package cmd

import (
	"fmt"

	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/client"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/docstrings"
)

//TODO: Move all output to status styled begin/done updates

func newRestartCommand(client *client.Client) *Command {
	restartStrings := docstrings.Get("restart")
	restartCmd := BuildCommandKS(nil, runRestart, restartStrings, client, requireSession, requireAppNameAsArg)
	restartCmd.Args = cobra.RangeArgs(0, 1)

	return restartCmd
}

func runRestart(cmdctx *cmdctx.CmdContext) error {
	ctx := cmdctx.Command.Context()

	app, err := cmdctx.Client.API().RestartApp(ctx, cmdctx.AppName)
	if err != nil {
		return err
	}

	fmt.Printf("%s is being restarted\n", app.Name)
	return nil
}
