package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/superfly/flyctl/cmdctx"

	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/docstrings"
)

//TODO: Move all output to status styled begin/done updates

func newSuspendCommand() *Command {

	suspendStrings := docstrings.Get("suspend")

	suspendCmd := BuildCommandKS(nil, runSuspend, suspendStrings, os.Stdout, requireSession, requireAppNameAsArg)
	suspendCmd.Args = cobra.RangeArgs(0, 1)

	return suspendCmd
}

func runSuspend(ctx *cmdctx.CmdContext) error {
	// appName := ctx.Args[0]
	// fmt.Println(appName, len(ctx.Args))
	// if appName == "" {
	// 	appName = ctx.AppName
	// }
	appName := ctx.AppName

	_, err := ctx.Client.API().SuspendApp(appName)
	if err != nil {
		return err
	}

	appstatus, err := ctx.Client.API().GetAppStatus(appName, false)
	if err != nil {
		return err
	}

	fmt.Printf("%s is now %s\n", appstatus.Name, appstatus.Status)

	allocount := len(appstatus.Allocations)

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = fmt.Sprintf("Suspending %s with %d instances to stop ", appstatus.Name, allocount)
	s.Start()

	for allocount > 0 {
		plural := ""
		if allocount > 1 {
			plural = "s"
		}
		s.Prefix = fmt.Sprintf("Suspending %s with %d instance%s to stop ", appstatus.Name, allocount, plural)
		appstatus, err = ctx.Client.API().GetAppStatus(ctx.AppName, false)
		if err != nil {
			return err
		}
		allocount = len(appstatus.Allocations)
	}

	s.FinalMSG = fmt.Sprintf("Suspend complete - %s is now suspended with no running instances\n", appstatus.Name)
	s.Stop()

	return nil
}
