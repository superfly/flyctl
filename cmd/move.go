package cmd

import (
	"fmt"

	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/client"

	"github.com/AlecAivazis/survey/v2"
	"github.com/logrusorgru/aurora"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/docstrings"
)

//TODO: Move all output to status styled begin/done updates

func newMoveCommand(client *client.Client) *Command {

	moveStrings := docstrings.Get("move")
	moveCmd := BuildCommandKS(nil, runMove, moveStrings, client, requireSession)
	moveCmd.Args = cobra.ExactArgs(1)
	// TODO: Move flag descriptions into the docStrings
	moveCmd.AddBoolFlag(BoolFlagOpts{Name: "yes", Shorthand: "y", Description: "Accept all confirmations"})
	moveCmd.AddStringFlag(StringFlagOpts{
		Name:        "org",
		Description: `The organization to move the app to`,
	})

	return moveCmd
}

func runMove(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()
	appName := cmdCtx.Args[0]

	app, err := cmdCtx.Client.API().GetApp(ctx, appName)
	if err != nil {
		return errors.Wrap(err, "Error fetching app")
	}

	cmdCtx.Statusf("move", cmdctx.SINFO, "App '%s' is currently in organization '%s'\n", app.Name, app.Organization.Slug)

	targetOrgSlug := cmdCtx.Config.GetString("org")
	org, err := selectOrganization(ctx, cmdCtx.Client.API(), targetOrgSlug, nil)

	switch {
	case isInterrupt(err):
		return nil
	case err != nil || org == nil:
		return fmt.Errorf("Error setting organization: %s", err)
	}

	if !cmdCtx.Config.GetBool("yes") {
		fmt.Println(aurora.Red(`Moving an app between organizations requires a complete shutdown and restart. This will result in some app downtime.
If the app relies on other services within the current organization, it may not come back up in a healthy manner.
Please confirm you wish to restart this app now?`))

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

	_, err = cmdCtx.Client.API().MoveApp(ctx, appName, org.ID)
	if err != nil {
		return errors.WithMessage(err, "Failed to move app")
	}

	fmt.Printf("Successfully moved %s to %s\n", appName, org.Slug)

	return nil
}
