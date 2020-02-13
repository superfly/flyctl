package cmd

import (
	"fmt"
	"github.com/AlecAivazis/survey/v2"
	"github.com/logrusorgru/aurora"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/flyctl"
	"os"
	"strconv"
)

func newAppListCommand() *Command {

	appsStrings := docstrings.Get("apps")

	cmd := &Command{
		Command: &cobra.Command{
			Use:   appsStrings.Usage,
			Short: appsStrings.Short,
			Long:  appsStrings.Long,
		},
	}

	appsListStrings := docstrings.Get("apps.list")

	BuildCommand(cmd, runAppsList, appsListStrings.Usage, appsListStrings.Short, appsListStrings.Long, true, os.Stdout)

	appsCreateStrings := docstrings.Get("apps.create")

	create := BuildCommand(cmd, runAppsCreate, appsCreateStrings.Usage, appsCreateStrings.Short, appsCreateStrings.Long, true, os.Stdout)
	create.Args = cobra.RangeArgs(0, 1)

	// TODO: Move flag descriptions into the docStrings
	create.AddStringFlag(StringFlagOpts{
		Name:        "name",
		Description: "The app name to use",
	})

	create.AddStringFlag(StringFlagOpts{
		Name:        "org",
		Description: `The organization that will own the app`,
	})

	create.AddStringFlag(StringFlagOpts{
		Name:        "port",
		Shorthand:   "p",
		Description: "Internal port on application to connect to external services",
	})

	create.AddStringFlag(StringFlagOpts{
		Name:        "builder",
		Description: `The Cloud Native Buildpacks builder to use when deploying the app`,
	})

	appsDestroyStrings := docstrings.Get("apps.destroy")
	destroy := BuildCommand(cmd, runDestroyApp, appsDestroyStrings.Usage, appsDestroyStrings.Short, appsDestroyStrings.Long, true, os.Stdout)
	destroy.Args = cobra.ExactArgs(1)
	// TODO: Move flag descriptions into the docStrings
	destroy.AddBoolFlag(BoolFlagOpts{Name: "yes", Shorthand: "y", Description: "Accept all confirmations"})

	appsMoveStrings := docstrings.Get("apps.move")
	move := BuildCommand(cmd, runAppsMove, appsMoveStrings.Usage, appsMoveStrings.Short, appsMoveStrings.Long, true, os.Stdout)
	move.Args = cobra.ExactArgs(1)
	// TODO: Move flag descriptions into the docStrings
	move.AddBoolFlag(BoolFlagOpts{Name: "yes", Shorthand: "y", Description: "Accept all confirmations"})
	move.AddStringFlag(StringFlagOpts{
		Name:        "org",
		Description: `The organization to move the app to`,
	})

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
	var appName = ""
	var internalPort = 0

	if len(ctx.Args) > 0 {
		appName = ctx.Args[0]
	}

	configPort, _ := ctx.Config.GetString("port")

	// If ports set, validate
	if configPort != "" {
		var err error

		internalPort, err = strconv.Atoi(configPort)
		if err != nil {
			return fmt.Errorf(`-p ports must be numeric`)
		}
	}

	newAppConfig := flyctl.NewAppConfig()

	if builder, _ := ctx.Config.GetString("builder"); builder != "" {
		newAppConfig.Build = &flyctl.Build{Builder: builder}
	}

	name, _ := ctx.Config.GetString("name")

	if name != "" && appName != "" {
		return fmt.Errorf(`Two app names specified %s and %s. Select and specify only one.`, appName, name)
	}

	if name == "" && appName != "" {
		name = appName
	}

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
	newAppConfig.AppName = app.Name
	newAppConfig.Definition = app.Config.Definition

	if configPort != "" {
		newAppConfig.SetInternalPort(internalPort)
	}

	fmt.Println("New app created")

	err = ctx.RenderEx(&presenters.AppInfo{App: *app}, presenters.Options{HideHeader: true, Vertical: true})
	if err != nil {
		return err
	}

	if ctx.ConfigFile == "" {
		newCfgFile, err := flyctl.ResolveConfigFileFromPath(ctx.WorkingDir)
		if err != nil {
			return err
		}
		ctx.ConfigFile = newCfgFile
	}

	return writeAppConfig(ctx.ConfigFile, newAppConfig)
}

func runAppsMove(ctx *CmdContext) error {
	appName := ctx.Args[0]

	targetOrgSlug, _ := ctx.Config.GetString("org")
	org, err := selectOrganization(ctx.FlyClient, targetOrgSlug)

	switch {
	case isInterrupt(err):
		return nil
	case err != nil || org == nil:
		return fmt.Errorf("Error setting organization: %s", err)
	}

	app, err := ctx.FlyClient.GetApp(appName)
	if err != nil {
		return errors.Wrap(err, "Error fetching app")
	}

	if !ctx.Config.GetBool("yes") {
		fmt.Println(aurora.Red("Are you sure you want to move this app?"))

		confirm := false
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Move %s from %s to %s?", appName, app.Organization.Slug, org.Slug),
		}
		survey.AskOne(prompt, &confirm)

		if !confirm {
			return nil
		}
	}

	app, err = ctx.FlyClient.MoveApp(appName, org.ID)
	if err != nil {
		return errors.WithMessage(err, "Failed to move app")
	}

	fmt.Printf("Successfully moved %s to %s\n", appName, org.Slug)

	return nil
}
