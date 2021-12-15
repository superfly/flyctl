package cmd

import (
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
)

//TODO: Move all output to status styled begin/done updates

func runCreate(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	var appName = ""

	if len(cmdCtx.Args) > 0 {
		appName = cmdCtx.Args[0]
	}

	name := ""

	// If we aren't generating the name automatically, get the name from the command line or from a prompt
	if !cmdCtx.Config.GetBool("generate-name") {
		name = cmdCtx.Config.GetString("name")

		// App name was specified as a command argument and via the --name option
		if name != "" && appName != "" {
			return fmt.Errorf(`two app names specified %s and %s. Select and specify only one`, appName, name)
		}

		if name == "" && appName != "" {
			name = appName
		}

		fmt.Println()

		if name == "" {

			// Prompt the user for the app name
			inputName, err := inputAppName("", true)

			if err != nil {
				return err
			}

			name = inputName
		} else {
			fmt.Printf("Selected App Name: %s\n", name)
		}
	}

	fmt.Println()

	targetOrgSlug := cmdCtx.Config.GetString("org")
	org, err := selectOrganization(ctx, cmdCtx.Client.API(), targetOrgSlug, nil)

	switch {
	case isInterrupt(err):
		return nil
	case err != nil || org == nil:
		return fmt.Errorf("Error setting organization: %s", err)
	}

	fmt.Println()

	input := api.CreateAppInput{
		Name:           name,
		Runtime:        "FIRECRACKER",
		OrganizationID: org.ID,
	}

	// set network if flag is set
	network := cmdCtx.Config.GetString("network")
	if network != "" {
		input.Network = api.StringPointer(network)
	}

	// The creation magic happens here....
	app, err := cmdCtx.Client.API().CreateApp(ctx, input)
	if err != nil {
		return err
	}

	fmt.Printf("New app created: %s\n", app.Name)

	return nil
}
