package launch

import (
	"bufio"
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
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/command/postgres"
	"github.com/superfly/flyctl/internal/command/redis"
	"github.com/superfly/flyctl/internal/filemu"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/scanner"
	"github.com/superfly/flyctl/terminal"
	"github.com/superfly/graphql"
)

func New() (cmd *cobra.Command) {
	const (
		long  = `Create and configure a new app from source code or a Docker image.`
		short = long
	)

	cmd = command.New("launch", short, long, run, command.RequireSession, command.LoadAppConfigIfPresent)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		// Since launch can perform a deployment, we offer the full set of deployment flags for those using
		// the launch command in CI environments. We may want to rescind this decision down the line, because
		// the list of flags is long, but it follows from the precedent of already offering some deployment flags.
		// See a proposed 'flag grouping' feature in Viper that could help with DX: https://github.com/spf13/cobra/pull/1778
		deploy.CommonFlags,

		flag.Org(),
		flag.NoDeploy(),
		flag.Bool{
			Name:        "generate-name",
			Description: "Always generate a name for the app, without prompting",
		},
		flag.String{
			Name:        "path",
			Description: `Path to the app source root, where fly.toml file will be saved`,
			Default:     ".",
		},
		flag.String{
			Name:        "name",
			Description: `Name of the new app`,
		},
		flag.Bool{
			Name:        "copy-config",
			Description: "Use the configuration file if present without prompting",
			Default:     false,
		},
		flag.Bool{
			Name:        "dockerignore-from-gitignore",
			Description: "If a .dockerignore does not exist, create one from .gitignore files",
			Default:     false,
		},
		flag.Bool{
			Name:        "force-nomad",
			Description: "Use the Apps v1 platform built with Nomad",
			Default:     false,
		},
		flag.Bool{
			Name:        "force-machines",
			Description: "Use the Apps v2 platform built with Machines",
			Default:     false,
		},
		flag.Int{
			Name:        "internal-port",
			Description: "Set internal_port for all services in the generated fly.toml",
			Default:     -1,
		},
	)

	return
}

func run(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)
	client := client.FromContext(ctx).API()
	workingDir := flag.GetString(ctx, "path")

	// Determine the working directory
	if absDir, err := filepath.Abs(workingDir); err == nil {
		workingDir = absDir
	}

	appConfig := app.NewConfig()

	var importedConfig bool
	configFilePath := filepath.Join(workingDir, "fly.toml")

	if exists, _ := app.ConfigFileExistsAtPath(configFilePath); exists {
		cfg, err := app.LoadConfig(ctx, configFilePath)
		if err != nil {
			return err
		}

		var deployExisting bool

		if cfg.AppName != "" {
			fmt.Fprintln(io.Out, "An existing fly.toml file was found for app", cfg.AppName)
			deployExisting, err = shouldDeployExistingApp(ctx, cfg.AppName)
			if err != nil {
				return err
			}
			ctx = app.WithName(ctx, cfg.AppName)
		} else {
			fmt.Fprintln(io.Out, "An existing fly.toml file was found")
		}

		if deployExisting {
			fmt.Fprintln(io.Out, "App is not running, deploy...")
			return deploy.DeployWithConfig(ctx, cfg, deploy.DeployWithConfigArgs{
				ForceNomad:    flag.GetBool(ctx, "force-nomad"),
				ForceMachines: flag.GetBool(ctx, "force-machines"),
				ForceYes:      flag.GetBool(ctx, "now"),
			})
		}

		copyConfig := false

		if flag.GetBool(ctx, "copy-config") {
			copyConfig = true
		} else {
			copy, err := prompt.Confirm(ctx, "Would you like to copy its configuration to the new app?")
			if copy && err == nil {
				copyConfig = true
			}
		}

		if copyConfig {
			appConfig = cfg
			importedConfig = true
		}
	}

	fmt.Fprintln(io.Out, "Creating app in", workingDir)

	srcInfo := new(scanner.SourceInfo)
	config := new(scanner.ScannerConfig)

	// Detect if --copy-config and --now flags are set. If so, limited set of
	// fly.toml file updates. Helpful for deploying PRs when the project is
	// already setup and we only need fly.toml config changes.
	if flag.GetBool(ctx, "copy-config") && flag.GetBool(ctx, "now") {
		config.Mode = "clone"
	} else {
		config.Mode = "launch"
	}

	if img := flag.GetString(ctx, "image"); img != "" {
		fmt.Fprintln(io.Out, "Using image", img)
		appConfig.Build = &app.Build{
			Image: img,
		}
	} else if dockerfile := flag.GetString(ctx, "dockerfile"); dockerfile != "" {
		fmt.Fprintln(io.Out, "Using dockerfile", dockerfile)
		appConfig.Build = &app.Build{
			Dockerfile: dockerfile,
		}
	} else {
		fmt.Fprintln(io.Out, "Scanning source code")

		if si, err := scanner.Scan(workingDir, config); err != nil {
			return err
		} else {
			srcInfo = si
		}

		if srcInfo == nil {
			fmt.Fprintln(io.Out, aurora.Green("Could not find a Dockerfile, nor detect a runtime or framework from source code. Continuing with a blank app."))
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

			fmt.Fprintf(io.Out, "Detected %s %s app\n", article, aurora.Green(appType))

			if srcInfo.Builder != "" {
				fmt.Fprintln(io.Out, "Using the following build configuration:")
				fmt.Fprintln(io.Out, "\tBuilder:", srcInfo.Builder)
				if srcInfo.Buildpacks != nil && len(srcInfo.Buildpacks) > 0 {
					fmt.Fprintln(io.Out, "\tBuildpacks:", strings.Join(srcInfo.Buildpacks, " "))
				}

				appConfig.Build = &app.Build{
					Builder:    srcInfo.Builder,
					Buildpacks: srcInfo.Buildpacks,
				}
			}
		}
	}

	if srcInfo != nil {
		for _, f := range srcInfo.Files {
			path := filepath.Join(workingDir, f.Path)

			if helpers.FileExists(path) {
				if flag.GetBool(ctx, "now") {
					fmt.Fprintf(io.Out, "You specified --now, so not overwriting %s\n", path)
					continue
				}
				confirm, err := prompt.ConfirmOverwrite(ctx, path)
				if !confirm || err != nil {
					continue
				}
			}

			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return err
			}

			perms := 0o600

			if strings.Contains(string(f.Contents), "#!") {
				perms = 0o700
			}

			if err := os.WriteFile(path, f.Contents, fs.FileMode(perms)); err != nil {
				return err
			}
		}
	}

	// Prompt for an app name or fetch from flags
	appName := ""

	if !flag.GetBool(ctx, "generate-name") {
		appName = flag.GetString(ctx, "name")

		if appName == "" {
			// Prompt the user for the app name
			inputName, err := prompt.SelectAppName(ctx)
			if err != nil {
				return err
			}

			appName = inputName
		} else {
			fmt.Fprintf(io.Out, "Selected App Name: %s\n", appName)
		}
	}

	// Prompt for an org
	// TODO: determine if eager remote builder is still required here
	org, err := prompt.Org(ctx)
	if err != nil {
		return
	}

	// If we potentially are deploying, launch a remote builder to prepare for deployment.
	if !flag.GetBool(ctx, "no-deploy") {
		go imgsrc.EagerlyEnsureRemoteBuilder(ctx, client, org.Slug)
	}

	region, err := prompt.Region(ctx, prompt.RegionParams{
		Message: "Choose a region for deployment:",
	})
	if err != nil {
		return err
	}

	input := api.CreateAppInput{
		Name:            appName,
		OrganizationID:  org.ID,
		PreferredRegion: &region.Code,
	}

	createdApp, err := client.CreateApp(ctx, input)
	if err != nil {
		return err
	}
	if !importedConfig {
		if cfg, err := app.FromDefinition(&createdApp.Config.Definition); err != nil {
			return err
		} else {
			appConfig = cfg
		}
	}

	appConfig.AppName = createdApp.Name
	ctx = app.WithName(ctx, appConfig.AppName)

	internalPortFromFlag := flag.GetInt(ctx, "internal-port")
	if internalPortFromFlag > 0 {
		appConfig.SetInternalPort(internalPortFromFlag)
	}

	if srcInfo != nil {

		if srcInfo.Port > 0 {
			appConfig.SetInternalPort(srcInfo.Port)
		}

		if srcInfo.Concurrency != nil {
			appConfig.SetConcurrency(srcInfo.Concurrency["soft_limit"], srcInfo.Concurrency["hard_limit"])
		}

		for envName, envVal := range srcInfo.Env {
			if envVal == "APP_FQDN" {
				appConfig.SetEnvVariable(envName, createdApp.Name+".fly.dev")
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

	fmt.Fprintf(io.Out, "Created app %s in organization %s\n", createdApp.Name, org.Slug)

	adminLink := fmt.Sprintf("https://fly.io/apps/%s", createdApp.Name)
	appLink := fmt.Sprintf("%s.fly.dev", createdApp.Name)
	fmt.Fprintf(io.Out, "Admin URL: %s\nHostname: %s\n", adminLink, appLink)

	// If secrets are requested by the launch scanner, ask the user to input them
	if srcInfo != nil && len(srcInfo.Secrets) > 0 {
		secrets := make(map[string]string)
		keys := []string{}

		for _, secret := range srcInfo.Secrets {

			val := ""

			// If a secret should be a random default, just generate it without displaying
			// Otherwise, prompt to type it in
			if secret.Generate != nil {
				if val, err = secret.Generate(); err != nil {
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
			_, err := client.SetSecrets(ctx, createdApp.Name, secrets)
			if err != nil {
				return err
			}
			fmt.Fprintf(io.Out, "Set secrets on %s: %s\n", createdApp.Name, strings.Join(keys, ", "))
		}
	}

	// If volumes are requested by the launch scanner, create them
	if srcInfo != nil && len(srcInfo.Volumes) > 0 {
		for _, vol := range srcInfo.Volumes {

			appID, err := client.GetAppID(ctx, createdApp.Name)
			if err != nil {
				return err
			}

			volume, err := client.CreateVolume(ctx, api.CreateVolumeInput{
				AppID:     appID,
				Name:      vol.Source,
				Region:    region.Code,
				SizeGb:    1,
				Encrypted: true,
			})

			if err != nil {
				return err
			} else {
				fmt.Fprintf(io.Out, "Created a %dGB volume %s in the %s region\n", volume.SizeGb, volume.ID, region.Code)
			}

		}
	}

	options := make(map[string]bool)
	if !(srcInfo == nil || srcInfo.SkipDatabase || flag.GetBool(ctx, "no-deploy") || flag.GetBool(ctx, "now")) {
		confirmPg, err := prompt.Confirm(ctx, "Would you like to set up a Postgresql database now?")
		if confirmPg && err == nil {
			LaunchPostgres(ctx, createdApp, org, region)
			options["postgresql"] = true
		}

		confirmRedis, err := prompt.Confirm(ctx, "Would you like to set up an Upstash Redis database now?")
		if confirmRedis && err == nil {
			LaunchRedis(ctx, createdApp, org, region)
			options["redis"] = true
		}

		// Run any initialization commands required for Postgres if it was installed
		if confirmPg && len(srcInfo.PostgresInitCommands) > 0 {
			for _, cmd := range srcInfo.PostgresInitCommands {
				if cmd.Condition {
					if err := execInitCommand(ctx, cmd); err != nil {
						return err
					}
				}
			}
		}
	}

	// Invoke Callback, if any
	if srcInfo != nil && srcInfo.Callback != nil {
		if err = srcInfo.Callback(srcInfo, options); err != nil {
			return err
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

	// Attempt to create a .dockerignore from .gitignore
	determineDockerIgnore(ctx, workingDir)

	// Complete the appConfig
	if srcInfo != nil {

		if srcInfo.Port > 0 {
			appConfig.SetInternalPort(srcInfo.Port)
		}

		if srcInfo.Concurrency != nil {
			appConfig.SetConcurrency(srcInfo.Concurrency["soft_limit"], srcInfo.Concurrency["hard_limit"])
		}

		for envName, envVal := range srcInfo.Env {
			if envVal == "APP_FQDN" {
				appConfig.SetEnvVariable(envName, createdApp.Name+".fly.dev")
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

	// Append any requested Dockerfile entries
	if srcInfo != nil && len(srcInfo.DockerfileAppendix) > 0 {
		if err := appendDockerfileAppendix(srcInfo.DockerfileAppendix); err != nil {
			return fmt.Errorf("failed appending Dockerfile appendix: %w", err)
		}
	}

	if srcInfo != nil && len(srcInfo.BuildArgs) > 0 {
		appConfig.Build = &app.Build{}
		appConfig.Build.Args = srcInfo.BuildArgs
	}

	// Finally, write the config

	flyTomlPath := filepath.Join(workingDir, "fly.toml")
	if err = appConfig.WriteToDisk(ctx, flyTomlPath); err != nil {
		return err
	}
	// round trip config, because some magic happens to populate stuff like services
	reloadedAppConfig, err := app.LoadConfig(ctx, flyTomlPath)
	if err != nil {
		return err
	}
	ctx = app.WithConfig(ctx, reloadedAppConfig)

	if srcInfo == nil {
		return nil
	}

	if !flag.GetBool(ctx, "no-deploy") && !flag.GetBool(ctx, "now") && !flag.GetBool(ctx, "auto-confirm") && reloadedAppConfig.HasNonHttpAndHttpsStandardServices() {
		hasUdpService := reloadedAppConfig.HasUdpService()
		ipStuffStr := "a dedicated ipv4 address"
		if !hasUdpService {
			ipStuffStr = "dedicated ipv4 and ipv6 addresses"
		}
		confirmDedicatedIp, err := prompt.Confirmf(ctx, "Would you like to allocate %s now?", ipStuffStr)
		if confirmDedicatedIp && err == nil {
			v4Dedicated, err := client.AllocateIPAddress(ctx, createdApp.Name, "v4", "", nil, "")
			if err != nil {
				return err
			}
			fmt.Fprintf(io.Out, "Allocated dedicated ipv4: %s\n", v4Dedicated.Address)
			if !hasUdpService {
				v6Dedicated, err := client.AllocateIPAddress(ctx, createdApp.Name, "v6", "", nil, "")
				if err != nil {
					return err
				}
				fmt.Fprintf(io.Out, "Allocated dedicated ipv6: %s\n", v6Dedicated.Address)
			}
		}
	}

	// Notices from a launcher about its behavior that should always be displayed
	if srcInfo.Notice != "" {
		fmt.Fprintln(io.Out, srcInfo.Notice)
	}

	deployNow := false
	promptForDeploy := true

	if srcInfo.SkipDeploy || flag.GetBool(ctx, "no-deploy") {
		deployNow = false
		promptForDeploy = false
	}

	if flag.GetBool(ctx, "now") {
		deployNow = true
		promptForDeploy = false
	}

	if promptForDeploy {
		confirm, err := prompt.Confirm(ctx, "Would you like to deploy now?")
		if confirm && err == nil {
			deployNow = true
		}
	}

	if deployNow {
		return deploy.DeployWithConfig(ctx, appConfig, deploy.DeployWithConfigArgs{
			ForceNomad:    flag.GetBool(ctx, "force-nomad"),
			ForceMachines: flag.GetBool(ctx, "force-machines"),
			ForceYes:      flag.GetBool(ctx, "now"),
		})
	}

	// Alternative deploy documentation if our standard deploy method is not correct
	if srcInfo.DeployDocs != "" {
		fmt.Fprintln(io.Out, srcInfo.DeployDocs)
	} else {
		fmt.Fprintln(io.Out, "Your app is ready! Deploy with `flyctl deploy`")
	}

	return nil
}

func execInitCommand(ctx context.Context, command scanner.InitCommand) (err error) {
	io := iostreams.FromContext(ctx)

	binary, err := exec.LookPath(command.Command)
	if err != nil {
		return fmt.Errorf("%s not found in $PATH - make sure app dependencies are installed and try again", command.Command)
	}
	fmt.Fprintln(io.Out, command.Description)
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
	if f, err = os.OpenFile(dockerfilePath, os.O_APPEND|os.O_WRONLY, 0o600); err != nil {
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

func shouldDeployExistingApp(ctx context.Context, appName string) (bool, error) {
	client := client.FromContext(ctx).API()
	status, err := client.GetAppStatus(ctx, appName, false)
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

func createDockerignoreFromGitignores(root string, gitIgnores []string) (string, error) {
	dockerIgnore := filepath.Join(root, ".dockerignore")
	f, err := os.Create(dockerIgnore)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := f.Close(); err != nil {
			terminal.Debugf("error closing %s file after writing: %v\n", dockerIgnore, err)
		}
	}()

	firstHeaderWritten := false
	foundFlyDotToml := false
	linebreak := []byte("\n")
	for _, gitIgnore := range gitIgnores {
		gitF, err := os.Open(gitIgnore)
		defer func() {
			if err := gitF.Close(); err != nil {
				terminal.Debugf("error closing %s file after reading: %v\n", gitIgnore, err)
			}
		}()
		if err != nil {
			terminal.Debugf("error opening %s file: %v\n", gitIgnore, err)
			continue
		}
		relDir, err := filepath.Rel(root, filepath.Dir(gitIgnore))
		if err != nil {
			terminal.Debugf("error finding relative directory of %s relative to root %s: %v\n", gitIgnore, root, err)
			continue
		}
		relFile, err := filepath.Rel(root, gitIgnore)
		if err != nil {
			terminal.Debugf("error finding relative file of %s relative to root %s: %v\n", gitIgnore, root, err)
			continue
		}

		headerWritten := false
		scanner := bufio.NewScanner(gitF)
		for scanner.Scan() {
			line := scanner.Text()
			if !headerWritten {
				if !firstHeaderWritten {
					firstHeaderWritten = true
				} else {
					f.Write(linebreak)
				}
				_, err := f.WriteString(fmt.Sprintf("# flyctl launch added from %s\n", relFile))
				if err != nil {
					return "", err
				}
				headerWritten = true
			}
			var dockerIgnoreLine string
			if strings.TrimSpace(line) == "" {
				dockerIgnoreLine = ""
			} else if strings.HasPrefix(line, "#") {
				dockerIgnoreLine = line
			} else if strings.HasPrefix(line, "!/") {
				dockerIgnoreLine = fmt.Sprintf("!%s", filepath.Join(relDir, line[2:]))
			} else if strings.HasPrefix(line, "!") {
				dockerIgnoreLine = fmt.Sprintf("!%s", filepath.Join(relDir, "**", line[1:]))
			} else if strings.HasPrefix(line, "/") {
				dockerIgnoreLine = filepath.Join(relDir, line[1:])
			} else {
				dockerIgnoreLine = filepath.Join(relDir, "**", line)
			}
			if strings.Contains(dockerIgnoreLine, "fly.toml") {
				foundFlyDotToml = true
			}
			if _, err := f.WriteString(dockerIgnoreLine); err != nil {
				return "", err
			}
			if _, err := f.Write(linebreak); err != nil {
				return "", err
			}
		}
	}

	if !foundFlyDotToml {
		if _, err := f.WriteString("fly.toml"); err != nil {
			return "", err
		}
		if _, err := f.Write(linebreak); err != nil {
			return "", err
		}
	}

	return dockerIgnore, nil
}

func determineDockerIgnore(ctx context.Context, workingDir string) (err error) {
	io := iostreams.FromContext(ctx)
	dockerIgnore := ".dockerignore"
	gitIgnore := ".gitignore"
	allGitIgnores := scanner.FindGitignores(workingDir)
	createDockerignoreFromGitignore := false

	// An existing .dockerignore should always be used instead of .gitignore
	if helpers.FileExists(dockerIgnore) {
		terminal.Debugf("Found %s file. Will use when deploying to Fly.\n", dockerIgnore)
		return
	}

	// If we find .gitignore files, determine whether they should be converted to .dockerignore
	if len(allGitIgnores) > 0 {

		if flag.GetBool(ctx, "dockerignore-from-gitignore") {
			createDockerignoreFromGitignore = true
		} else {
			confirm, err := prompt.Confirm(ctx, fmt.Sprintf("Create %s from %d %s files?", dockerIgnore, len(allGitIgnores), gitIgnore))
			if confirm && err == nil {
				createDockerignoreFromGitignore = true
			}
		}

		if createDockerignoreFromGitignore {
			createdDockerIgnore, err := createDockerignoreFromGitignores(workingDir, allGitIgnores)
			if err != nil {
				terminal.Warnf("Error creating %s from %d %s files: %v\n", dockerIgnore, len(allGitIgnores), gitIgnore, err)
			} else {
				fmt.Fprintf(io.Out, "Created %s from %d %s files.\n", createdDockerIgnore, len(allGitIgnores), gitIgnore)
			}
			return nil
		}
	}
	return
}

func LaunchPostgres(ctx context.Context, app *api.App, org *api.Organization, region *api.Region) {
	io := iostreams.FromContext(ctx)
	clusterAppName := app.Name + "-db"
	err := postgres.CreateCluster(ctx, org, region, "machines",
		&postgres.ClusterParams{
			PostgresConfiguration: postgres.PostgresConfiguration{
				Name: clusterAppName,
			},
		})

	if err != nil {
		fmt.Fprintf(io.Out, "Failed creating the Postgres cluster %s: %s", clusterAppName, err)
	} else {
		err = postgres.AttachCluster(ctx, postgres.AttachParams{
			PgAppName: clusterAppName,
			AppName:   app.Name,
		})

		if err != nil {
			msg := `Failed attaching %s to the Postgres cluster %s: %w.\nTry attaching manually with 'fly postgres attach --app %s %s'`
			fmt.Fprintf(io.Out, msg, app.Name, clusterAppName, err, app.Name, clusterAppName)

		} else {
			fmt.Fprintf(io.Out, "Postgres cluster %s is now attached to %s\n", clusterAppName, app.Name)
		}
	}
}

func LaunchRedis(ctx context.Context, app *api.App, org *api.Organization, region *api.Region) {
	name := app.Name + "-redis"

	db, err := redis.Create(ctx, org, name, region, "", true, false)

	if err != nil {
		fmt.Println(fmt.Errorf("%w", err))
	} else {
		redis.AttachDatabase(ctx, db, app)
	}
}
