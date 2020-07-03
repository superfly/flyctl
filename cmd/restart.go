package cmd

import (
	"fmt"
	"os"

	"github.com/superfly/flyctl/cmdctx"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/docstrings"
)

//TODO: Move all output to status styled begin/done updates

func newRestartCommand() *Command {
	restartStrings := docstrings.Get("restart")
	restartCmd := BuildCommand(nil, runRestart, restartStrings.Usage, restartStrings.Short, restartStrings.Long, os.Stdout, requireSession, requireAppNameAsArg)
	restartCmd.Args = cobra.RangeArgs(0, 1)

	return restartCmd
}

func runRestart(ctx *cmdctx.CmdContext) error {
	app, err := ctx.Client.API().RestartApp(ctx.AppName)
	if err != nil {
		return err
	}

	fmt.Printf("%s is being restarted\n", app.Name)
	return nil
}
