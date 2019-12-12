package cmd

import (
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/docker"
	"github.com/superfly/flyctl/flyctl"
)

func newAppListCommand() *Command {
	cmd := &Command{
		Command: &cobra.Command{
			Use:   "apps",
			Short: "manage apps",
			Long:  "Manage your Fly applications. You'll usually start with the \"create\" subcommand.",
		},
	}

	BuildCommand(cmd, runAppsList, "list", "list apps", os.Stdout, true)

	create := BuildCommand(cmd, runAppsCreate, "create", "create a new app", os.Stdout, true)
	create.AddStringFlag(StringFlagOpts{
		Name:        "name",
		Description: "the app name to use",
	})
	create.AddStringFlag(StringFlagOpts{
		Name:        "org",
		Description: `the organization that will own the app`,
	})
	create.AddStringFlag(StringFlagOpts{
		Name:        "builder",
		Description: `the builder to use when deploying the app`,
	})

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
			Message: fmt.Sprintf("Destroy app %s?", appName),
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

func runAppsCreate(ctx *CmdContext) error {
	newAppConfig := flyctl.NewAppConfig()

	if namedBuilder, _ := ctx.Config.GetString("builder"); namedBuilder != "" {
		url, err := docker.ResolveNamedBuilderURL(namedBuilder)
		if err == docker.ErrUnknownBuilder {
			return fmt.Errorf(`Unknown builder "%s". See %s for a list of builders.`, namedBuilder, docker.BuildersRepo)
		}
		newAppConfig.Build = &flyctl.Build{Builder: url}
	}

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
	newAppConfig.AppName = name

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
	newAppConfig.Definition = app.Config.Definition

	fmt.Println("New app created")

	err = ctx.RenderEx(&presenters.AppInfo{App: *app}, presenters.Options{HideHeader: true, Vertical: true})
	if err != nil {
		return err
	}

	return writeAppConfig(ctx.ConfigFile, newAppConfig)
}
