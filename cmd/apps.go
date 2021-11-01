package cmd

import (
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/client"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/docstrings"
)

func newAppsCommand(client *client.Client) *Command {

	appsStrings := docstrings.Get("apps")

	cmd := &Command{
		Command: &cobra.Command{
			Use:   appsStrings.Usage,
			Short: appsStrings.Short,
			Long:  appsStrings.Long,
		},
	}

	appsListStrings := docstrings.Get("apps.list")

	BuildCommand(cmd, runAppsList, appsListStrings.Usage, appsListStrings.Short, appsListStrings.Long, client, requireSession)

	appsCreateStrings := docstrings.Get("apps.create")

	create := BuildCommand(cmd, runCreate, appsCreateStrings.Usage, appsCreateStrings.Short, appsCreateStrings.Long, client, requireSession)
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

	create.AddBoolFlag(BoolFlagOpts{
		Name:        "generate-name",
		Description: "Always generate a name for the app",
	})

	create.AddStringFlag(StringFlagOpts{
		Name:        "network",
		Description: "Specify custom network id",
	})

	appsDestroyStrings := docstrings.Get("apps.destroy")
	destroy := BuildCommand(cmd, runDestroy, appsDestroyStrings.Usage, appsDestroyStrings.Short, appsDestroyStrings.Long, client, requireSession)
	destroy.Args = cobra.ExactArgs(1)
	// TODO: Move flag descriptions into the docStrings
	destroy.AddBoolFlag(BoolFlagOpts{Name: "yes", Shorthand: "y", Description: "Accept all confirmations"})

	appsMoveStrings := docstrings.Get("apps.move")
	move := BuildCommand(cmd, runMove, appsMoveStrings.Usage, appsMoveStrings.Short, appsMoveStrings.Long, client, requireSession)
	move.Args = cobra.ExactArgs(1)
	// TODO: Move flag descriptions into the docStrings
	move.AddBoolFlag(BoolFlagOpts{Name: "yes", Shorthand: "y", Description: "Accept all confirmations"})
	move.AddStringFlag(StringFlagOpts{
		Name:        "org",
		Description: `The organization to move the app to`,
	})

	appsSuspendStrings := docstrings.Get("apps.suspend")
	appsSuspendCmd := BuildCommand(cmd, runSuspend, appsSuspendStrings.Usage, appsSuspendStrings.Short, appsSuspendStrings.Long, client, requireSession, requireAppNameAsArg)
	appsSuspendCmd.Args = cobra.RangeArgs(0, 1)

	appsResumeStrings := docstrings.Get("apps.resume")
	appsResumeCmd := BuildCommand(cmd, runResume, appsResumeStrings.Usage, appsResumeStrings.Short, appsResumeStrings.Long, client, requireSession, requireAppNameAsArg)
	appsResumeCmd.Args = cobra.RangeArgs(0, 1)

	appsRestartStrings := docstrings.Get("apps.restart")
	appsRestartCmd := BuildCommand(cmd, runRestart, appsRestartStrings.Usage, appsRestartStrings.Short, appsRestartStrings.Long, client, requireSession, requireAppNameAsArg)
	appsRestartCmd.Args = cobra.RangeArgs(0, 1)

	return cmd
}

func runAppsList(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	listapps, err := cmdCtx.Client.API().GetApps(ctx, nil)
	if err != nil {
		return err
	}

	return cmdCtx.Render(&presenters.Apps{Apps: listapps})
}
