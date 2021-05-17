package cmd

import (
	"fmt"
	"strconv"

	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/client"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
)

//TODO: Move all output to status styled begin/done updates

func newInitCommand(client *client.Client) *Command {

	initStrings := docstrings.Get("init")

	cmd := BuildCommandKS(nil, runInit, initStrings, client, requireSession)

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

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "port",
		Shorthand:   "p",
		Description: "Internal port on application to connect to external services",
	})

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "builder",
		Description: `The Cloud Native Buildpacks builder to use when deploying the app`,
	})

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "builtin",
		Description: `The Fly Runtime to use for building the app`,
	})

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "image",
		Description: `Deploy this named image`,
	})

	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "dockerfile",
		Description: `Use a dockerfile when deploying the app`,
		Default:     false,
	})

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "import",
		Description: "Create but import all settings from the given file",
	})

	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "overwrite",
		Description: "Always silently overwrite an existing fly.toml file",
	})

	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "nowrite",
		Description: "Never write a fly.toml file",
	})

	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "generatename",
		Description: "Always generate a name for the app", Hidden: true,
	})

	return cmd
}

func runInit(cmdCtx *cmdctx.CmdContext) error {
	var appName = ""
	var internalPort = 0

	if len(cmdCtx.Args) > 0 {
		appName = cmdCtx.Args[0]
	}

	configPort := cmdCtx.Config.GetString("port")

	// If ports set, validate
	if configPort != "" {
		var err error

		internalPort, err = strconv.Atoi(configPort)
		if err != nil {
			return fmt.Errorf(`-p ports must be numeric`)
		}
	}
	overwrite := cmdCtx.Config.GetBool("overwrite")
	nowrite := cmdCtx.Config.GetBool("nowrite") || cmdCtx.Config.GetBool("no-config")

	configfilename, err := flyctl.ResolveConfigFileFromPath(cmdCtx.WorkingDir)
	if err != nil {
		return err
	}

	newAppConfig := flyctl.NewAppConfig()

	if !nowrite {
		if helpers.FileExists(configfilename) {
			if !overwrite {
				cmdCtx.Status("init", cmdctx.SERROR, "An existing configuration file has been found.")
				confirmation := confirm(fmt.Sprintf("Overwrite file '%s'", configfilename))
				if !confirmation {
					return nil
				}
			}
		}
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
	org, err := selectOrganization(cmdCtx.Client.API(), targetOrgSlug)

	switch {
	case isInterrupt(err):
		return nil
	case err != nil || org == nil:
		return fmt.Errorf("Error setting organization: %s", err)
	}

	fmt.Println()

	builder := cmdCtx.Config.GetString("builder")
	builtinname := cmdCtx.Config.GetString("builtin")
	importfile := cmdCtx.Config.GetString("import")
	imagename := cmdCtx.Config.GetString("image")

	if !nowrite {
		// If we are importing or using a builtin, assume builders are set in the template
		if importfile == "" && builtinname == "" && imagename == "" {
			// Otherwise get a Builder from the user while checking the dockerfile setting
			dockerfileSet := cmdCtx.Config.IsSet("dockerfile")
			dockerfile := cmdCtx.Config.GetBool("dockerfile")

			if builder == "" && !dockerfileSet {
				builder, builtin, err := selectBuildtype(cmdCtx)

				switch {
				case isInterrupt(err):
					return nil
				case err != nil || builder == "":
					return fmt.Errorf("Error setting builder: %s", err)
				}
				// If image, prompt for name
				if builder == "Image" {
					imagename, err = selectImage(cmdCtx)
					if err != nil {
						return err
					}
				} else if builder != "Dockerfile" && builder != "None" && !builtin {
					// Not a dockerfile setting and not set to none. This is a classic buildpack
					newAppConfig.Build = &flyctl.Build{Builder: builder}
				} else if builder != "None" && builtin {
					// Builder not none and the user apparently selected a builtin builder
					builtinname = builder
				}
			} else if builder != "" {
				// If the builder was set and there's not dockerfile setting, write the builder
				if !dockerfile {
					newAppConfig.Build = &flyctl.Build{Builder: builder}
				}
			}
		}
	}
	// The creation magic happens here....
	app, err := cmdCtx.Client.API().CreateApp(name, org.ID, nil)
	if err != nil {
		return err
	}

	if !nowrite {
		if imagename != "" {
			newAppConfig.AppName = app.Name
			newAppConfig.Build = &flyctl.Build{Image: imagename}
			newAppConfig.Definition = app.Config.Definition
		} else if importfile != "" {
			if !cmdCtx.OutputJSON() {
				fmt.Printf("Importing configuration from %s\n", importfile)
			}

			tmpappconfig, err := flyctl.LoadAppConfig(importfile)
			if err != nil {
				return err
			}
			newAppConfig = tmpappconfig
			// And then overwrite the app name
			newAppConfig.AppName = app.Name
		} else if builtinname != "" {
			newAppConfig.AppName = app.Name
			newAppConfig.Build = &flyctl.Build{Builtin: builtinname}
			newAppConfig.Definition = app.Config.Definition
		} else if builder != "None" {
			newAppConfig.AppName = app.Name
			newAppConfig.Definition = app.Config.Definition
		}

		if configPort != "" { // If the config port has been set externally, set that
			newAppConfig.SetInternalPort(internalPort)
		} else if importfile != "" {
			if newAppConfig.HasServices() {
				currentport, err := newAppConfig.GetInternalPort()
				if err != nil {
					return err
				}
				if !cmdCtx.OutputJSON() {
					fmt.Printf("Importing port %d\n", currentport)
				}
			}
		} else if builtinname != "" {
			if !cmdCtx.OutputJSON() {
				fmt.Printf("Builtins use port 8080\n")
			}
			newAppConfig.SetInternalPort(8080)
		} else {
			// If we are not importing and not running a builtin, get the default, ask for new setting
			currentport, err := newAppConfig.GetInternalPort()
			if err != nil {
				return err
			}
			internalPort, err = selectPort(cmdCtx, currentport)
			if err != nil {
				return err
			}
			newAppConfig.SetInternalPort(internalPort)
		}

		if cmdCtx.OutputJSON() {
			cmdCtx.WriteJSON(app)
			return nil
		}

		err = cmdCtx.Frender(cmdctx.PresenterOption{Presentable: &presenters.AppInfo{App: *app}, HideHeader: true, Vertical: true, Title: "New app created"})
		if err != nil {
			return err
		}

		fmt.Printf("App will initially deploy to %s (%s) region\n\n", (*app.Regions)[0].Code, (*app.Regions)[0].Name)
		if cmdCtx.ConfigFile == "" {
			newCfgFile, err := flyctl.ResolveConfigFileFromPath(cmdCtx.WorkingDir)
			if err != nil {
				return err
			}
			cmdCtx.ConfigFile = newCfgFile
		}

		cmdCtx.AppName = app.Name
		cmdCtx.AppConfig = newAppConfig

		return writeAppConfig(cmdCtx.ConfigFile, newAppConfig)
	}

	fmt.Printf("New app created: %s", app.Name)

	return nil
}
