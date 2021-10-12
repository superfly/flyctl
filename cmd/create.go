package cmd

import (
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"

	"github.com/AlecAivazis/survey/v2"
)

//TODO: Move all output to status styled begin/done updates

func runCreate(cmdCtx *cmdctx.CmdContext) error {
	var appName = ""

	if len(cmdCtx.Args) > 0 {
		appName = cmdCtx.Args[0]
	}

	name := ""

	if !cmdCtx.Config.GetBool("generate-name") {
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
	app, err := cmdCtx.Client.API().CreateApp(input)
	if err != nil {
		return err
	}

	fmt.Printf("New app created: %s\n", app.Name)

	return nil
}
