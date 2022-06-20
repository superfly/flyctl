package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
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
	"github.com/superfly/flyctl/internal/filemu"
	"github.com/superfly/flyctl/internal/sourcecode"
	"github.com/superfly/graphql"
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
	launchCmd.AddBoolFlag(BoolFlagOpts{
		Name:        "remote-only",
		Description: "Perform builds remotely without using the local docker daemon",
		Default:     true,
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

	var srcInfo = new(sourcecode.SourceInfo)

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

			appType := srcInfo.Family

			if srcInfo.Version != "" {
				appType = appType + " " + srcInfo.Version
			}

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

			perms := 0600

			if strings.Contains(string(f.Contents), "#!") {
				perms = 0700
			}

			if err := os.WriteFile(path, f.Contents, fs.FileMode(perms)); err != nil {
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
			if envVal == "APP_FQDN" {
				appConfig.SetEnvVariable(envName, app.Name+".fly.dev")
			} else {
				appConfig.SetEnvVariable(envName, envVal)
			}
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

		if srcInfo.DockerEntrypoint != "" {
			appConfig.SetDockerEntrypoint(srcInfo.DockerEntrypoint)
		}

		if srcInfo.KillSignal != "" {
			appConfig.SetKillSignal(srcInfo.KillSignal)
		}
	}

	fmt.Printf("Created app %s in organization %s\n", cmdCtx.AppName, org.Slug)

	// If secrets are requested by the launch scanner, ask the user to input them
	if srcInfo != nil && len(srcInfo.Secrets) > 0 {
		secrets := make(map[string]string)
		keys := []string{}

		for _, secret := range srcInfo.Secrets {

			val := ""

			// If a secret should be a random default, just generate it without displaying
			// Otherwise, prompt to type it in
			if secret.Generate {
				if val, err = helpers.RandString(64); err != nil {
					return fmt.Errorf("could not generate random string: %w", err)
				}

			} else if secret.Value != "" {
				val = secret.Value
			} else {
				prompt := fmt.Sprintf("Set secret %s:", secret.Key)

				surveyInput := &survey.Input{
					Message: prompt,
					Help:    secret.Help,
				}

				survey.AskOne(surveyInput, &val)
			}

			if val != "" {
				secrets[secret.Key] = val
				keys = append(keys, secret.Key)
			}
		}

		if len(secrets) > 0 {
			_, err := cmdCtx.Client.API().SetSecrets(ctx, cmdCtx.AppName, secrets)

			if err != nil {
				return err
			}
			fmt.Printf("Set secrets on %s: %s\n", cmdCtx.AppName, strings.Join(keys, ", "))
		}
	}

	// If volumes are requested by the launch scanner, create them
	if srcInfo != nil && len(srcInfo.Volumes) > 0 {

		for _, vol := range srcInfo.Volumes {

			appID, err := cmdCtx.Client.API().GetAppID(ctx, cmdCtx.AppName)

			if err != nil {
				return err
			}

			volume, err := cmdCtx.Client.API().CreateVolume(ctx, api.CreateVolumeInput{
				AppID:     appID,
				Name:      vol.Source,
				Region:    region.Code,
				SizeGb:    1,
				Encrypted: true,
			})

			if err != nil {
				return err
			} else {
				fmt.Printf("Created a %dGB volume %s in the %s region\n", volume.SizeGb, volume.ID, region.Code)
			}

		}
	}

	// Run any initialization commands
	if srcInfo != nil && len(srcInfo.InitCommands) > 0 {
		for _, cmd := range srcInfo.InitCommands {
			if err := execInitCommand(ctx, cmd); err != nil {
				return err
			}
		}
	}

	// Append any requested Dockerfile entries
	if srcInfo != nil && len(srcInfo.DockerfileAppendix) > 0 {
		if err := appendDockerfileAppendix(srcInfo.DockerfileAppendix); err != nil {
			return fmt.Errorf("failed appending Dockerfile appendix: %w", err)
		}
	}

	if srcInfo != nil && len(srcInfo.BuildArgs) > 0 {
		appConfig.Build = &flyctl.Build{}
		appConfig.Build.Args = srcInfo.BuildArgs
	}

	// Finally, write the config
	if err := writeAppConfig(filepath.Join(dir, "fly.toml"), appConfig); err != nil {
		return err
	}

	if srcInfo == nil {
		return nil
	}

	if !cmdCtx.Config.GetBool("no-deploy") && !cmdCtx.Config.GetBool("now") && !srcInfo.SkipDatabase && confirm("Would you like to setup a Postgresql database now?") {

		appID, err := cmdCtx.Client.API().GetAppID(ctx, cmdCtx.AppName)

		if err != nil {
			return err
		}

		clusterAppName := cmdCtx.AppName + "-db"

		cmdCtx.Config.Set("name", clusterAppName)
		cmdCtx.Config.Set("region", region.Code)
		cmdCtx.Config.Set("organization", org.Slug)

		err = runCreatePostgresCluster(cmdCtx)

		if err != nil {
			err = fmt.Errorf("failed creating the Postgres cluster %s: %w", clusterAppName, err)
			return err
		}

		cmdCtx.Config.Set("postgres-app", clusterAppName)

		// Reset the app name here beacuse runCreatePostgresCluster overrides it
		cmdCtx.AppName = appID
		err = runAttachPostgresCluster(cmdCtx)

		// Reset the app name here beacuse AttachPostgresCluster overrides it
		cmdCtx.AppName = appID

		if err != nil {
			msg := `Failed attaching %s to the Postgres cluster %s: %w.\nTry attaching manually with 'fly postgres attach --app %s --postgres-app %s'`
			err = fmt.Errorf(msg, clusterAppName, appID, err, appID, clusterAppName)
			return err
		}

		fmt.Printf("Postgres cluster %s is now attached to %s\n", clusterAppName, cmdCtx.AppName)

		// Run any initialization commands required for postgres support
		if len(srcInfo.PostgresInitCommands) > 0 {
			for _, cmd := range srcInfo.PostgresInitCommands {
				if cmd.Condition {
					if err := execInitCommand(ctx, cmd); err != nil {
						return err
					}
				}
			}
		}

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

func execInitCommand(ctx context.Context, command sourcecode.InitCommand) (err error) {
	binary, err := exec.LookPath(command.Command)
	if err != nil {
		return fmt.Errorf("%s not found in $PATH - make sure app dependencies are installed and try again", command.Command)
	}
	fmt.Println(command.Description)
	// Run a requested generator command, for example to generate a Dockerfile
	cmd := exec.CommandContext(ctx, binary, command.Args...)

	if err = cmd.Start(); err != nil {
		return err
	}

	if err = cmd.Wait(); err != nil {
		err = fmt.Errorf("failed running %s: %w ", cmd.String(), err)
	}
	return err
}

func appendDockerfileAppendix(appendix []string) (err error) {
	const dockerfilePath = "Dockerfile"

	var b bytes.Buffer
	b.WriteString("\n# Appended by flyctl\n")

	for _, value := range appendix {
		_, _ = b.WriteString(value)
		_ = b.WriteByte('\n')
	}

	var unlock filemu.UnlockFunc

	if unlock, err = filemu.Lock(context.Background(), dockerfilePath); err != nil {
		return
	}
	defer func() {
		if e := unlock(); err == nil {
			err = e
		}
	}()

	var f *os.File
	// TODO: we don't flush
	if f, err = os.OpenFile(dockerfilePath, os.O_APPEND|os.O_WRONLY, 0600); err != nil {
		return
	}
	defer func() {
		if e := f.Close(); err == nil {
			err = e
		}
	}()

	_, err = b.WriteTo(f)

	return
}

func shouldDeployExistingApp(cmdCtx *cmdctx.CmdContext, appName string) (bool, error) {
	ctx := cmdCtx.Command.Context()

	status, err := cmdCtx.Client.API().GetAppStatus(ctx, appName, false)
	if err != nil {
		if api.IsNotFoundError(err) || graphql.IsNotFoundError(err) {
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
