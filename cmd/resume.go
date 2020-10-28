package cmd

import (
	"fmt"
	"os"

	"github.com/superfly/flyctl/cmdctx"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/docstrings"
)

//TODO: Move all output to status styled begin/done updates

func newResumeCommand() *Command {

	resumeStrings := docstrings.Get("resume")
	resumeCmd := BuildCommandKS(nil, runResume, resumeStrings, os.Stdout, requireSession, requireAppNameAsArg)
	resumeCmd.Args = cobra.RangeArgs(0, 1)

	return resumeCmd
}

func runResume(cmdctx *cmdctx.CmdContext) error {
	app, err := cmdctx.Client.API().ResumeApp(cmdctx.AppName)
	if err != nil {
		return err
	}

	fmt.Printf("%s is now %s\n", app.Name, app.Status)

	return nil
}
