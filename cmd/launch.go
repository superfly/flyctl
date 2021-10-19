package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/sourcecode"

	. "github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/docstrings"
)

func newLaunchCommand(client *client.Client) *Command {
	launchStrings := docstrings.Get("launch")
	launchCmd := BuildCommandKS(nil, runLaunch, launchStrings, client, requireSession)
	launchCmd.Args = cobra.NoArgs
	launchCmd.AddStringFlag(StringFlagOpts{
		Name:        "path",
		Description: `path to app code and where a fly.toml file will be saved.`,
		Default:     "."},
	)
	launchCmd.AddStringFlag(StringFlagOpts{
		Name:        "org",
		Description: `the organization that will own the app`,
	})
	launchCmd.AddStringFlag(StringFlagOpts{
		Name:        "name",
		Description: "the name of the new app",
	})
	launchCmd.AddStringFlag(StringFlagOpts{
		Name:        "region",
		Description: "the region to launch the new app in",
	})
	launchCmd.AddStringFlag(StringFlagOpts{
		Name:        "image",
		Description: "the image to launch",
	})
	launchCmd.AddBoolFlag(BoolFlagOpts{
		Name:        "now",
		Description: "deploy now without confirmation",
		Default:     false,
	})
	launchCmd.AddBoolFlag(BoolFlagOpts{
		Name:        "no-deploy",
		Description: "Do not prompt for deployment",
		Default:     false,
	})
	launchCmd.AddBoolFlag(BoolFlagOpts{
		Name:        "generate-name",
		Description: "Always generate a name for the app",
		Default:     false,
	})
	launchCmd.AddStringFlag(StringFlagOpts{
		Name:        "dockerfile",
		Description: "Path to a Dockerfile. Defaults to the Dockerfile in the working directory.",
	})

	return launchCmd
}

func runLaunch(cmdctx *cmdctx.CmdContext) error {
	dir := cmdctx.Config.GetString("path")

	if absDir, err := filepath.Abs(dir); err == nil {
		dir = absDir
	}
	cmdctx.WorkingDir = dir

	orgSlug := cmdctx.Config.GetString("org")

	// start a remote builder for the personal org if necessary
	eagerBuilderOrg := orgSlug
	if orgSlug == "" {
		eagerBuilderOrg = "personal"
	}
	go imgsrc.EagerlyEnsureRemoteBuilder(cmdctx.Client.API(), eagerBuilderOrg)

	appConfig := flyctl.NewAppConfig()

	var importedConfig bool
	configFilePath := filepath.Join(dir, "fly.toml")
	if exists, _ := flyctl.ConfigFileExistsAtPath(configFilePath); exists {
		cfg, err := flyctl.LoadAppConfig(configFilePath)
		if err != nil {
			return err
		}

		var deployExisting bool

		if cfg.AppName != "" {
			fmt.Println("An existing fly.toml file was found for app", cfg.AppName)
			deployExisting, err = shouldDeployExistingApp(cmdctx, cfg.AppName)
			if err != nil {
				return err
			}
		} else {
			fmt.Println("An existing fly.toml file was found")
		}

		if deployExisting {
			fmt.Println("App is not running, deploy...")
			cmdctx.AppName = cfg.AppName
			cmdctx.AppConfig = cfg
			return runDeploy(cmdctx)
		} else if confirm("Would you like to copy its configuration to the new app?") {
			appConfig.Definition = cfg.Definition
			importedConfig = true
		}
	}

	fmt.Println("Creating app in", dir)
	var srcInfo *sourcecode.SourceInfo

	if img := cmdctx.Config.GetString("image"); img != "" {
		fmt.Println("Using image", img)
		appConfig.Build = &flyctl.Build{
			Image: img,
		}
	} else if dockerfile := cmdctx.Config.GetString("dockerfile"); dockerfile != "" {
		fmt.Println("Using dockefile", dockerfile)
		appConfig.Build = &flyctl.Build{
			Dockerfile: dockerfile,
		}
	} else {
		fmt.Println("Scanning source code")

		if si, err := sourcecode.Scan(dir); err != nil {
			return err
		} else {
			srcInfo = si
		}

		if srcInfo == nil {
			fmt.Println(Green("Could not find a Dockerfile, nor detect a runtime or framework from source code. Continuing with a blank app."))
		} else {

			var article string = "a"
			matched, _ := regexp.MatchString(`^[aeiou]`, strings.ToLower(srcInfo.Family))

			if matched {
				article += "n"
			}

			fmt.Printf("Detected %s %s app\n", article, Green(srcInfo.Family))

			if srcInfo.Builder != "" {
				fmt.Println("Using the following build configuration:")
				fmt.Println("\tBuilder:", srcInfo.Builder)
				if srcInfo.Buildpacks != nil && len(srcInfo.Buildpacks) > 0 {
					fmt.Println("\tBuildpacks:", strings.Join(srcInfo.Buildpacks, " "))
				}

				appConfig.Build = &flyctl.Build{
					Builder:    srcInfo.Builder,
					Buildpacks: srcInfo.Buildpacks,
				}
			}
		}
	}

	if srcInfo != nil {
		for _, f := range srcInfo.Files {
			path := filepath.Join(dir, f.Path)

			if helpers.FileExists(path) && !confirmOverwrite(path) {
				continue
			}

			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return err
			}

			if err := os.WriteFile(path, f.Contents, 0666); err != nil {
				return err
			}
		}
	}

	appName := ""

	if !cmdctx.Config.GetBool("generate-name") {
		appName = cmdctx.Config.GetString("name")

		if appName == "" {
			// Prompt the user for the app name
			inputName, err := inputAppName("", true)

			if err != nil {
				return err
			}

			appName = inputName
		} else {
			fmt.Printf("Selected App Name: %s\n", appName)
		}
	}

	org, err := selectOrganization(cmdctx.Client.API(), orgSlug, nil)
	if err != nil {
		return err
	}

	// spawn another builder if the chosen org is different
	if org.Slug != eagerBuilderOrg {
		go imgsrc.EagerlyEnsureRemoteBuilder(cmdctx.Client.API(), org.Slug)
	}

	regionCode := cmdctx.Config.GetString("region")
	region, err := selectRegion(cmdctx.Client.API(), regionCode)
	if err != nil {
		return err
	}

	app, err := cmdctx.Client.API().CreateApp(appName, org.ID, &region.Code)
	if err != nil {
		return err
	}
	if !importedConfig {
		appConfig.Definition = app.Config.Definition
	}

	cmdctx.AppName = app.Name
	appConfig.AppName = app.Name
	cmdctx.AppConfig = appConfig

	if srcInfo != nil {
		if srcInfo.Port > 0 {
			appConfig.SetInternalPort(srcInfo.Port)
		}

		for envName, envVal := range srcInfo.Env {
			appConfig.SetEnvVariable(envName, envVal)
		}

		if len(srcInfo.Statics) > 0 {
			appConfig.SetStatics(srcInfo.Statics)
		}

		for procName, procCommand := range srcInfo.Processes {
			appConfig.SetProcess(procName, procCommand)
		}
	}

	fmt.Printf("Created app %s in organization %s\n", app.Name, org.Slug)

	if srcInfo != nil && len(srcInfo.Secrets) > 0 {
		secrets := make(map[string]string)
		keys := []string{}

		for k, v := range srcInfo.Secrets {
			val := ""
			prompt := fmt.Sprintf("Set secret %s:", k)
			survey.AskOne(&survey.Input{
				Message: prompt,
				Help:    v,
			}, &val)

			if val != "" {
				secrets[k] = val
				keys = append(keys, k)
			}
		}

		if len(secrets) > 0 {
			_, err := cmdctx.Client.API().SetSecrets(app.Name, secrets)

			if err != nil {
				return err
			}
			fmt.Printf("Set secrets on %s: %s\n", app.Name, strings.Join(keys, ", "))
		}
	}

	if err := writeAppConfig(filepath.Join(dir, "fly.toml"), appConfig); err != nil {
		return err
	}

	if srcInfo == nil {
		return nil
	}

	fmt.Println("Your app is ready. Deploy with `flyctl deploy`")

	if !cmdctx.Config.GetBool("no-deploy") && (cmdctx.Config.GetBool("now") || confirm("Would you like to deploy now?")) {
		return runDeploy(cmdctx)
	}

	return nil
}

func shouldDeployExistingApp(cc *cmdctx.CmdContext, appName string) (bool, error) {
	status, err := cc.Client.API().GetAppStatus(appName, false)
	if err != nil {
		if api.IsNotFoundError(err) || err.Error() == "Could not resolve App" {
			return false, nil
		}
		return false, err
	}

	if !status.Deployed {
		return true, nil
	}

	for _, a := range status.Allocations {
		if a.Healthy {
			return false, nil
		}
	}

	return true, nil
}
