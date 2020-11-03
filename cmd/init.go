package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/superfly/flyctl/cmdctx"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
)

//TODO: Move all output to status styled begin/done updates

func newInitCommand() *Command {

	initStrings := docstrings.Get("init")

	cmd := BuildCommandKS(nil, runInit, initStrings, os.Stdout, requireSession)

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
		Description: "Always overwrite an existing fly.toml file",
	})

	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "nowrite",
		Description: "Never write a fly.toml file",
	})

	return cmd
}

func runInit(commandContext *cmdctx.CmdContext) error {
	var appName = ""
	var internalPort = 0

	if len(commandContext.Args) > 0 {
		appName = commandContext.Args[0]
	}

	configPort, _ := commandContext.Config.GetString("port")

	// If ports set, validate
	if configPort != "" {
		var err error

		internalPort, err = strconv.Atoi(configPort)
		if err != nil {
			return fmt.Errorf(`-p ports must be numeric`)
		}
	}
	overwrite := commandContext.Config.GetBool("overwrite")
	nowrite := commandContext.Config.GetBool("nowrite")

	configfilename, err := flyctl.ResolveConfigFileFromPath(commandContext.WorkingDir)
	if err != nil {
		return err
	}

	newAppConfig := flyctl.NewAppConfig()

	if !nowrite {
		if helpers.FileExists(configfilename) {
			if !overwrite {
				commandContext.Status("init", cmdctx.SERROR, "An existing configuration file has been found.")
				confirmation := confirm(fmt.Sprintf("Overwrite file '%s'", configfilename))
				if !confirmation {
					return nil
				}
			} else {
				commandContext.Status("init", cmdctx.SWARN, "Overwriting existing configuration (--overwrite)")
			}
		}
	}

	name, _ := commandContext.Config.GetString("name")

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

	fmt.Println()

	targetOrgSlug, _ := commandContext.Config.GetString("org")
	org, err := selectOrganization(commandContext.Client.API(), targetOrgSlug)

	switch {
	case isInterrupt(err):
		return nil
	case err != nil || org == nil:
		return fmt.Errorf("Error setting organization: %s", err)
	}

	fmt.Println()

	builder, err := commandContext.Config.GetString("builder")
	if err != nil {
		return err
	}

	builtinname, _ := commandContext.Config.GetString("builtin")
	if err != nil {
		return err
	}

	importfile, err := commandContext.Config.GetString("import")
	if err != nil {
		return err
	}

	imagename, err := commandContext.Config.GetString("image")
	if err != nil {
		return err
	}

	if !nowrite {
		// If we are importing or using a builtin, assume builders are set in the template
		if importfile == "" && builtinname == "" && imagename == "" {
			// Otherwise get a Builder from the user while checking the dockerfile setting
			dockerfileSet := commandContext.Config.IsSet("dockerfile")
			dockerfile := commandContext.Config.GetBool("dockerfile")

			if builder == "" && !dockerfileSet {
				builder, builtin, err := selectBuildtype(commandContext)

				switch {
				case isInterrupt(err):
					return nil
				case err != nil || builder == "":
					return fmt.Errorf("Error setting builder: %s", err)
				}
				// If image, prompt for name
				if builder == "Image" {
					imagename, err = selectImage(commandContext)
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
	app, err := commandContext.Client.API().CreateApp(name, org.ID)
	if err != nil {
		return err
	}

	if !nowrite {
		if imagename != "" {
			newAppConfig.AppName = app.Name
			newAppConfig.Build = &flyctl.Build{Image: imagename}
			newAppConfig.Definition = app.Config.Definition
		} else if importfile != "" {
			fmt.Printf("Importing configuration from %s\n", importfile)

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
			currentport, err := newAppConfig.GetInternalPort()
			if err != nil {
				return err
			}
			fmt.Printf("Importing port %d\n", currentport)
		} else if builtinname != "" {
			fmt.Printf("Builtins use port 8080\n")
			newAppConfig.SetInternalPort(8080)
		} else {
			// If we are not importing and not running a builtin, get the default, ask for new setting
			currentport, err := newAppConfig.GetInternalPort()
			if err != nil {
				return err
			}
			internalPort, err = selectPort(commandContext, currentport)
			if err != nil {
				return err
			}
			newAppConfig.SetInternalPort(internalPort)
		}

		fmt.Println()

		err = commandContext.Frender(cmdctx.PresenterOption{Presentable: &presenters.AppInfo{App: *app}, HideHeader: true, Vertical: true, Title: "New app created"})
		if err != nil {
			return err
		}

		fmt.Printf("App will initially deploy to %s (%s) region\n\n", (*app.Regions)[0].Code, (*app.Regions)[0].Name)
		if commandContext.ConfigFile == "" {
			newCfgFile, err := flyctl.ResolveConfigFileFromPath(commandContext.WorkingDir)
			if err != nil {
				return err
			}
			commandContext.ConfigFile = newCfgFile
		}

		commandContext.AppName = app.Name
		commandContext.AppConfig = newAppConfig

		return writeAppConfig(commandContext.ConfigFile, newAppConfig)
	}

	fmt.Printf("New app created: %s", app.Name)

	f, err := os.OpenFile("fly.alias", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return err
	}

	defer f.Close()

	if _, err = f.WriteString(app.Name + "\n"); err != nil {
		return err
	}

	return nil
}
