package cmd

import (
	"fmt"

	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/client"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/docstrings"
)

//TODO: Move all output to status styled begin/done updates

func newCreateCommand(client *client.Client) *Command {

	initStrings := docstrings.Get("create")

	cmd := BuildCommandKS(nil, runCreate, initStrings, client, requireSession)

	cmd.Args = cobra.RangeArgs(0, 1)

	// TODO: Move flag descriptions into the docStrings
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "name",
		Description: "The app name to use",
	})

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "org",
		Description: `The organization that will own the app`,
	})

	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "generatename",
		Description: "Always generate a name for the app", Hidden: true,
	})

	return cmd
}

func runCreate(cmdCtx *cmdctx.CmdContext) error {
	var appName = ""

	if len(cmdCtx.Args) > 0 {
		appName = cmdCtx.Args[0]
	}

	name := ""

	if !cmdCtx.Config.GetBool("generatename") {
		name = cmdCtx.Config.GetString("name")

		if name != "" && appName != "" {
			return fmt.Errorf(`two app names specified %s and %s. Select and specify only one`, appName, name)
		}

		if name == "" && appName != "" {
			name = appName
		}

		fmt.Println()

		if name == "" {
			prompt := &survey.Input{
				Message: "App Name (leave blank to use an auto-generated name)",
			}
			if err := survey.AskOne(prompt, &name); err != nil {
				if isInterrupt(err) {
					return nil
				}
			}
		} else {
			fmt.Printf("Selected App Name: %s\n", name)
		}
	}

	fmt.Println()

	targetOrgSlug := cmdCtx.Config.GetString("org")
	org, err := selectOrganization(cmdCtx.Client.API(), targetOrgSlug, nil)

	switch {
	case isInterrupt(err):
		return nil
	case err != nil || org == nil:
		return fmt.Errorf("Error setting organization: %s", err)
	}

	fmt.Println()

	// The creation magic happens here....
	app, err := cmdCtx.Client.API().CreateApp(name, org.ID, nil)
	if err != nil {
		return err
	}

	fmt.Printf("New app created: %s\n", app.Name)

	return nil
}
