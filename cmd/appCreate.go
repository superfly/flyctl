package cmd

import (
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/flyctl"
)

func newAppCreateCommand() *Command {
	cmd := BuildCommand(nil, runAppCreate, "create", "create a new app", os.Stdout, true)
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "name",
		Description: "the app name to use",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "org",
		Description: `the organization that will own the app`,
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "builder",
		Description: `the builder to use when deploying this app`,
	})

	return cmd
}

func isInterrupt(err error) bool {
	return err != nil && err.Error() == "interrupt"
}

func runAppCreate(ctx *CmdContext) error {
	name, _ := ctx.Config.GetString("name")
	if name == "" {
		prompt := &survey.Input{
			Message: "App Name (leave blank to use an auto-generated name)",
		}
		if err := survey.AskOne(prompt, &name); err != nil {
			if isInterrupt(err) {
				return nil
			}
		}
	}

	targetOrgSlug, _ := ctx.Config.GetString("org")
	org, err := selectOrganization(ctx.FlyClient, targetOrgSlug)

	switch {
	case isInterrupt(err):
		return nil
	case err != nil || org == nil:
		return fmt.Errorf("Error setting organization: %s", err)
	}

	app, err := ctx.FlyClient.CreateApp(name, org.ID)
	if err != nil {
		return err
	}

	fmt.Println("New app created")

	err = ctx.RenderEx(&presenters.AppInfo{App: *app}, presenters.Options{HideHeader: true, Vertical: true})
	if err != nil {
		return err
	}

	p := flyctl.NewProject(".")
	p.SetAppName(app.Name)
	if builder, _ := ctx.Config.GetString("builder"); builder != "" {
		p.SetBuilder(builder)
	}

	if err := p.SafeWriteConfig(); err != nil {
		fmt.Printf(
			"Configure flyctl by placing the following in a fly.toml file:\n\n%s\n\n",
			p.WriteConfigAsString(),
		)
	} else {
		fmt.Println("Created fly.toml")
	}

	return nil
}
