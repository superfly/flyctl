package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/client"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/docstrings"
)

//TODO: Move all output to status styled begin/done updates

func newResumeCommand(client *client.Client) *Command {
	ctxOptions := map[string]interface{}{
		"requireAppNameAsArg": true,
	}
	resumeStrings := docstrings.Get("resume")
	resumeCmd := BuildCommandKS(nil, runResume, resumeStrings, client, ctxOptions, requireSession, requireAppName)
	resumeCmd.Args = cobra.RangeArgs(0, 1)

	return resumeCmd
}

func runResume(cmdctx *cmdctx.CmdContext) error {
	app, err := cmdctx.Client.API().ResumeApp(cmdctx.AppName)
	if err != nil {
		return err
	}

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = fmt.Sprintf("Resuming %s with 1 instance to start ", cmdctx.AppName)
	s.Start()

	for app.Status != "running" {
		app, err = cmdctx.Client.API().GetApp(cmdctx.AppName)
		if err != nil {
			return err
		}
	}

	s.FinalMSG = fmt.Sprintf("Resume complete - %s is now %s with 1 running instance\n", cmdctx.AppName, app.Status)
	s.Stop()

	return nil
}
