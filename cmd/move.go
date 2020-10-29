package cmd

import (
	"fmt"
	"os"

	"github.com/superfly/flyctl/cmdctx"

	"github.com/AlecAivazis/survey/v2"
	"github.com/logrusorgru/aurora"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/docstrings"
)

//TODO: Move all output to status styled begin/done updates

func newMoveCommand() *Command {

	moveStrings := docstrings.Get("move")
	moveCmd := BuildCommandKS(nil, runMove, moveStrings, os.Stdout, requireSession)
	moveCmd.Args = cobra.ExactArgs(1)
	// TODO: Move flag descriptions into the docStrings
	moveCmd.AddBoolFlag(BoolFlagOpts{Name: "yes", Shorthand: "y", Description: "Accept all confirmations"})
	moveCmd.AddStringFlag(StringFlagOpts{
		Name:        "org",
		Description: `The organization to move the app to`,
	})

	return moveCmd
}

func runMove(commandContext *cmdctx.CmdContext) error {
	appName := commandContext.Args[0]

	app, err := commandContext.Client.API().GetApp(appName)
	if err != nil {
		return errors.Wrap(err, "Error fetching app")
	}

	commandContext.Statusf("move", cmdctx.SINFO, "App '%s' is currently in organization '%s'\n", app.Name, app.Organization.Slug)

	targetOrgSlug, _ := commandContext.Config.GetString("org")
	org, err := selectOrganization(commandContext.Client.API(), targetOrgSlug)

	switch {
	case isInterrupt(err):
		return nil
	case err != nil || org == nil:
		return fmt.Errorf("Error setting organization: %s", err)
	}

	if !commandContext.Config.GetBool("yes") {
		fmt.Println(aurora.Red("Are you sure you want to move this app?"))

		confirm := false
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Move %s from %s to %s?", appName, app.Organization.Slug, org.Slug),
		}
		err = survey.AskOne(prompt, &confirm)
		if err != nil {
			return err
		}
		if !confirm {
			return nil
		}
	}

	_, err = commandContext.Client.API().MoveApp(appName, org.ID)
	if err != nil {
		return errors.WithMessage(err, "Failed to move app")
	}

	fmt.Printf("Successfully moved %s to %s\n", appName, org.Slug)

	return nil
}
