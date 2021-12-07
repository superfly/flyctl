package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/sourcecode"
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
	launchCmd.AddBoolFlag(BoolFlagOpts{
		Name:        "copy-config",
		Description: "Use the configuration file if present without prompting.",
		Default:     false,
	})

	return launchCmd
}

func runLaunch(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	dir := cmdCtx.Config.GetString("path")

	if absDir, err := filepath.Abs(dir); err == nil {
		dir = absDir
	}
	cmdCtx.WorkingDir = dir

	orgSlug := cmdCtx.Config.GetString("org")

	// start a remote builder for the personal org if necessary
	eagerBuilderOrg := orgSlug
	if orgSlug == "" {
		eagerBuilderOrg = "personal"
	}
	go imgsrc.EagerlyEnsureRemoteBuilder(ctx, cmdCtx.Client.API(), eagerBuilderOrg)

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
			deployExisting, err = shouldDeployExistingApp(cmdCtx, cfg.AppName)
			if err != nil {
				return err
			}
		} else {
			fmt.Println("An existing fly.toml file was found")
		}

		if deployExisting {
			fmt.Println("App is not running, deploy...")
			cmdCtx.AppName = cfg.AppName
			cmdCtx.AppConfig = cfg
			return runDeploy(cmdCtx)
		} else if cmdCtx.Config.GetBool("copy-config") || confirm("Would you like to copy its configuration to the new app?") {
			appConfig.Definition = cfg.Definition
			importedConfig = true
		}
	}

	fmt.Println("Creating app in", dir)
	var srcInfo *sourcecode.SourceInfo

	if img := cmdCtx.Config.GetString("image"); img != "" {
		fmt.Println("Using image", img)
		appConfig.Build = &flyctl.Build{
			Image: img,
		}
	} else if dockerfile := cmdCtx.Config.GetString("dockerfile"); dockerfile != "" {
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
			fmt.Println(aurora.Green("Could not find a Dockerfile, nor detect a runtime or framework from source code. Continuing with a blank app."))
		} else {

			var article string = "a"
			matched, _ := regexp.MatchString(`^[aeiou]`, strings.ToLower(srcInfo.Family))

			if matched {
				article += "n"
			}

			appType := srcInfo.Family + " " + srcInfo.Version
			fmt.Printf("Detected %s %s app\n", article, aurora.Green(appType))

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

	if !cmdCtx.Config.GetBool("generate-name") {
		appName = cmdCtx.Config.GetString("name")

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

	org, err := selectOrganization(ctx, cmdCtx.Client.API(), orgSlug, nil)
	if err != nil {
		return err
	}

	// spawn another builder if the chosen org is different
	if org.Slug != eagerBuilderOrg {
		go imgsrc.EagerlyEnsureRemoteBuilder(ctx, cmdCtx.Client.API(), org.Slug)
	}

	regionCode := cmdCtx.Config.GetString("region")
	region, err := selectRegion(ctx, cmdCtx.Client.API(), regionCode)
	if err != nil {
		return err
	}

	input := api.CreateAppInput{
		Name:            appName,
		OrganizationID:  org.ID,
		PreferredRegion: &region.Code,
		Runtime:         "FIRECRACKER",
	}

	app, err := cmdCtx.Client.API().CreateApp(ctx, input)
	if err != nil {
		return err
	}
	if !importedConfig {
		appConfig.Definition = app.Config.Definition
	}

	cmdCtx.AppName = app.Name
	appConfig.AppName = app.Name
	cmdCtx.AppConfig = appConfig

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

		if len(srcInfo.Volumes) > 0 {
			appConfig.SetVolumes(srcInfo.Volumes)
		}

		for procName, procCommand := range srcInfo.Processes {
			appConfig.SetProcess(procName, procCommand)
		}

		if srcInfo.ReleaseCmd != "" {
			appConfig.SetReleaseCommand(srcInfo.ReleaseCmd)
		}

		if srcInfo.DockerCommand != "" {
			appConfig.SetDockerCommand(srcInfo.DockerCommand)
		}

		if srcInfo.DockerCommand != "" {
			appConfig.SetDockerEntrypoint(srcInfo.DockerEntrypoint)
		}
	}

	fmt.Printf("Created app %s in organization %s\n", app.Name, org.Slug)

	// If secrets are requested by the launch scanner, ask the user to input them
	if srcInfo != nil && len(srcInfo.Secrets) > 0 {
		secrets := make(map[string]string)
		keys := []string{}

		for k, v := range srcInfo.Secrets {
			val := ""
			prompt := fmt.Sprintf("Set secret %s:", k)

			surveyInput := &survey.Input{
				Message: prompt,
				Help:    v,
			}

			if strings.Contains(v, "random default") {
				surveyInput.Default, err = helpers.RandString(64)
				if err != nil {
					return err
				}

			}

			survey.AskOne(surveyInput, &val)

			if val != "" {
				secrets[k] = val
				keys = append(keys, k)
			}
		}

		if len(secrets) > 0 {
			_, err := cmdCtx.Client.API().SetSecrets(ctx, app.Name, secrets)

			if err != nil {
				return err
			}
			fmt.Printf("Set secrets on %s: %s\n", app.Name, strings.Join(keys, ", "))
		}
	}

	// If volumes are requested by the launch scanner, create them
	if srcInfo != nil && len(srcInfo.Volumes) > 0 {

		for _, vol := range srcInfo.Volumes {

			app, err := cmdCtx.Client.API().GetApp(ctx, cmdCtx.AppName)

			if err != nil {
				return err
			}

			volume, err := cmdCtx.Client.API().CreateVolume(ctx, api.CreateVolumeInput{
				AppID:     app.ID,
				Name:      vol.Source,
				Region:    region.Code,
				SizeGb:    10,
				Encrypted: true,
			})

			if err != nil {
				return err
			} else {
				fmt.Printf("Created a %dGB volume %s in the %s region\n", volume.SizeGb, volume.ID, region.Code)
			}

		}
	}

	if srcInfo.InitCommand != "" {
		binary, err := exec.LookPath(srcInfo.InitCommand)
		if err != nil {
			return fmt.Errorf("%s not found - make sure app dependencies are installed and try again: %w", srcInfo.InitCommand, err)
		}

		// Run a requested generator command, for example to generate a Dockerfile
		cmd := exec.CommandContext(ctx, binary, srcInfo.InitCommandArgs...)

		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err = cmd.Start(); err != nil {
			return err
		}

		if err = cmd.Wait(); err != nil {
			err = fmt.Errorf("failed running: %s ", cmd.String())

			return err
		}
	}
	// Append any requested Dockerfile entries

	if srcInfo.DockerfileAppendix != nil {
		f, err := os.OpenFile("Dockerfile", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)

		if err != nil {
			return err
		}

		defer f.Close()

		for _, value := range srcInfo.DockerfileAppendix {
			if _, err = f.WriteString(value + "\n"); err != nil {
				return err
			}
		}
	}

	// Finally, write the config
	if err := writeAppConfig(filepath.Join(dir, "fly.toml"), appConfig); err != nil {
		return err
	}

	if srcInfo == nil {
		return nil
	}

	// Notices from a launcher about its behavior that should always be displayed
	if srcInfo.Notice != "" {
		fmt.Println(srcInfo.Notice)
	}

	if !cmdCtx.Config.GetBool("no-deploy") &&
		!srcInfo.SkipDeploy &&
		(cmdCtx.Config.GetBool("now") || confirm("Would you like to deploy now?")) {
		return runDeploy(cmdCtx)
	}

	// Alternative deploy documentation if our standard deploy method is not correct
	if srcInfo.DeployDocs != "" {
		fmt.Println(srcInfo.DeployDocs)
	} else {
		fmt.Println("Your app is ready. Deploy with `flyctl deploy`")
	}

	return nil
}

func shouldDeployExistingApp(cmdCtx *cmdctx.CmdContext, appName string) (bool, error) {
	ctx := cmdCtx.Command.Context()

	status, err := cmdCtx.Client.API().GetAppStatus(ctx, appName, false)
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
