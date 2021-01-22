package cmd

import (
	"os"

	"github.com/superfly/flyctl/cmdctx"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/docstrings"
)

func newAppsCommand() *Command {

	appsStrings := docstrings.Get("apps")

	cmd := &Command{
		Command: &cobra.Command{
			Use:   appsStrings.Usage,
			Short: appsStrings.Short,
			Long:  appsStrings.Long,
		},
	}

	appsListStrings := docstrings.Get("apps.list")

	BuildCommand(cmd, runAppsList, appsListStrings.Usage, appsListStrings.Short, appsListStrings.Long, os.Stdout, requireSession)

	appsCreateStrings := docstrings.Get("apps.create")

	create := BuildCommand(cmd, runInit, appsCreateStrings.Usage, appsCreateStrings.Short, appsCreateStrings.Long, os.Stdout, requireSession)
	create.Args = cobra.RangeArgs(0, 1)

	// TODO: Move flag descriptions into the docStrings
	create.AddStringFlag(StringFlagOpts{
		Name:        "name",
		Description: "The app name to use",
	})

	create.AddStringFlag(StringFlagOpts{
		Name:        "org",
		Description: `The organization that will own the app`,
	})

	create.AddStringFlag(StringFlagOpts{
		Name:        "port",
		Shorthand:   "p",
		Description: "Internal port on application to connect to external services",
	})

	create.AddStringFlag(StringFlagOpts{
		Name:        "builder",
		Description: `The Cloud Native Buildpacks builder to use when deploying the app`,
	})

	appsDestroyStrings := docstrings.Get("apps.destroy")
	destroy := BuildCommand(cmd, runDestroy, appsDestroyStrings.Usage, appsDestroyStrings.Short, appsDestroyStrings.Long, os.Stdout, requireSession)
	destroy.Args = cobra.ExactArgs(1)
	// TODO: Move flag descriptions into the docStrings
	destroy.AddBoolFlag(BoolFlagOpts{Name: "yes", Shorthand: "y", Description: "Accept all confirmations"})

	appsMoveStrings := docstrings.Get("apps.move")
	move := BuildCommand(cmd, runMove, appsMoveStrings.Usage, appsMoveStrings.Short, appsMoveStrings.Long, os.Stdout, requireSession)
	move.Args = cobra.ExactArgs(1)
	// TODO: Move flag descriptions into the docStrings
	move.AddBoolFlag(BoolFlagOpts{Name: "yes", Shorthand: "y", Description: "Accept all confirmations"})
	move.AddStringFlag(StringFlagOpts{
		Name:        "org",
		Description: `The organization to move the app to`,
	})

	appsSuspendStrings := docstrings.Get("apps.suspend")
	appsSuspendCmd := BuildCommand(cmd, runSuspend, appsSuspendStrings.Usage, appsSuspendStrings.Short, appsSuspendStrings.Long, os.Stdout, requireSession, requireAppNameAsArg)
	appsSuspendCmd.Args = cobra.RangeArgs(0, 1)

	appsResumeStrings := docstrings.Get("apps.resume")
	appsResumeCmd := BuildCommand(cmd, runResume, appsResumeStrings.Usage, appsResumeStrings.Short, appsResumeStrings.Long, os.Stdout, requireSession, requireAppNameAsArg)
	appsResumeCmd.Args = cobra.RangeArgs(0, 1)

	appsRestartStrings := docstrings.Get("apps.restart")
	appsRestartCmd := BuildCommand(cmd, runRestart, appsRestartStrings.Usage, appsRestartStrings.Short, appsRestartStrings.Long, os.Stdout, requireSession, requireAppNameAsArg)
	appsRestartCmd.Args = cobra.RangeArgs(0, 1)

	return cmd
}

func runAppsList(ctx *cmdctx.CmdContext) error {
	listapps, err := ctx.Client.API().GetApps(nil)
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.Apps{Apps: listapps})
}
