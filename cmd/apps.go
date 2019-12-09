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
	"github.com/superfly/flyctl/helpers"
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

	initConfig := BuildCommand(cmd, runAppInit, "init-config [APP] [PATH]", "initialize a fly.toml file from an existing app", os.Stdout, true)
	initConfig.Args = cobra.RangeArgs(1, 2)

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
	builder := ""

	if namedBuilder, _ := ctx.Config.GetString("builder"); namedBuilder != "" {
		url, err := docker.ResolveNamedBuilderURL(namedBuilder)
		if err == docker.ErrUnknownBuilder {
			return fmt.Errorf(`Unknown builder "%s". See %s for a list of builders.`, namedBuilder, docker.BuildersRepo)
		}
		builder = url
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

	project, err := initConfigFromApp(ctx, app.Name, ".")
	if err != nil {
		return err
	}

	if builder != "" {
		project.SetBuilder(builder)
	}

	return writeConfigWithPrompt(project)
}

func runAppInit(ctx *CmdContext) error {
	appName := ctx.Args[0]

	path := "."
	if len(ctx.Args) == 2 {
		path = ctx.Args[1]
	}

	project, err := initConfigFromApp(ctx, appName, path)
	if err != nil {
		return err
	}

	return writeConfigWithPrompt(project)
}

func initConfigFromApp(ctx *CmdContext, appName, path string) (*flyctl.Project, error) {
	path, err := flyctl.ResolveConfigFileFromPath(path)
	if err != nil {
		return nil, err
	}

	app, err := ctx.FlyClient.GetApp(appName)
	if err != nil {
		return nil, err
	}

	services, err := ctx.FlyClient.GetAppServices(appName)
	if err != nil {
		return nil, err
	}

	project := flyctl.NewProject(path)
	project.SetAppName(app.Name)
	project.SetServices(services)

	return project, nil
}

func writeConfigWithPrompt(project *flyctl.Project) error {

	if exists, _ := flyctl.ConfigFileExistsAtPath(project.ConfigFilePath()); exists {
		if !confirm(fmt.Sprintf("Overwrite config file '%s'", project.ConfigFilePath())) {
			return nil
		}
	}

	if err := project.WriteConfig(); err != nil {
		return err
	}

	// Commented to silence the console echo of the config file
	//fmt.Println(aurora.Faint(project.WriteConfigAsString()))

	path := helpers.PathRelativeToCWD(project.ConfigFilePath())
	fmt.Println("Wrote config file", path)

	return nil
}
