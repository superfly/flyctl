package cmd

import (
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppListCommand() *Command {
	cmd := &Command{
		Command: &cobra.Command{
			Use:   "apps",
			Short: "manage apps",
			Long:  "manage apps",
		},
	}

	BuildCommand(cmd, runAppsList, "list", "list apps", os.Stdout, true)
	delete := BuildCommand(cmd, runDestroyApp, "destroy", "permanently destroy an app", os.Stdout, true)
	delete.Args = cobra.ExactArgs(1)
	delete.AddBoolFlag(BoolFlagOpts{Name: "yes", Shorthand: "y", Description: "accept all confirmations"})

	return cmd
}

func runAppsList(ctx *CmdContext) error {
	apps, err := ctx.FlyClient.GetApps()
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.Apps{Apps: apps})
}

func runDestroyApp(ctx *CmdContext) error {
	appName := ctx.Args[0]

	if !ctx.Config.GetBool("yes") {
		fmt.Println(aurora.Red("Destroying an app is not reversible."))

		confirm := false
		prompt := &survey.Confirm{
			Message: "Destroy app?",
		}
		survey.AskOne(prompt, &confirm)

		if !confirm {
			return nil
		}
	}

	if err := ctx.FlyClient.DeleteApp(appName); err != nil {
		return err
	}

	fmt.Println("Destroyed app", appName)

	return nil
}
