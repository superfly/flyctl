package cmd

import (
	"fmt"
	"os"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
)

func newInitCommand() *Command {
	init := BuildCommand(nil, runAppInit, "init [PATH]", "initialize a fly.toml file from an app", os.Stdout, true, requireAppName)
	init.Args = cobra.MaximumNArgs(1)

	return init
}

func runAppInit(ctx *CmdContext) error {
	path := "."
	if len(ctx.Args) == 1 {
		path = ctx.Args[0]
	}

	project, err := initConfigFromApp(ctx, ctx.AppName(), path)
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

	cfgServices := []flyctl.Service{}

	for _, s := range services {
		cfgServices = append(cfgServices, flyctl.Service{
			Protocol:     s.Protocol,
			Port:         s.Port,
			InternalPort: s.InternalPort,
			Handlers:     s.Handlers,
		})
	}

	project.SetServices(cfgServices)

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

	fmt.Println(aurora.Faint(project.WriteConfigAsString()))

	path := helpers.PathRelativeToCWD(project.ConfigFilePath())
	fmt.Println("Wrote config file", path)

	return nil
}
