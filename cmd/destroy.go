package cmd

import (
	"fmt"

	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/client"

	"github.com/AlecAivazis/survey/v2"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/docstrings"
)

//TODO: Move all output to status styled begin/done updates

func newDestroyCommand(client *client.Client) *Command {

	destroyStrings := docstrings.Get("destroy")

	destroy := BuildCommand(nil, runDestroy, destroyStrings.Usage, destroyStrings.Short, destroyStrings.Long, client, nil, requireSession)

	destroy.Args = cobra.ExactArgs(1)

	destroy.AddBoolFlag(BoolFlagOpts{Name: "yes", Shorthand: "y", Description: "Accept all confirmations"})

	return destroy
}

func runDestroy(ctx *cmdctx.CmdContext) error {
	appName := ctx.Args[0]

	if !ctx.Config.GetBool("yes") {
		fmt.Println(aurora.Red("Destroying an app is not reversible."))

		confirm := false
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Destroy app %s?", appName),
		}
		err := survey.AskOne(prompt, &confirm)

		if err != nil {
			return err
		}

		if !confirm {
			return nil
		}
	}

	if err := ctx.Client.API().DeleteApp(appName); err != nil {
		return err
	}

	fmt.Println("Destroyed app", appName)

	return nil
}
