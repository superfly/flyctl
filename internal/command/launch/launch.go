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
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/appv2"
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

	var importedConfig bool
	appConfig := app.NewConfig()

	configFilePath := filepath.Join(workingDir, "fly.toml")
	if exists, _ := flyctl.ConfigFileExistsAtPath(configFilePath); exists {
		cfg, err := app.LoadConfig(ctx, configFilePath, "nomad")
		if err != nil {
			return err
		}

		if cfg.AppName != "" {
			fmt.Fprintln(io.Out, "An existing fly.toml file was found for app", cfg.AppName)
			if deployExisting, err := shouldDeployExistingApp(ctx, cfg.AppName); err != nil {
				return err
			} else if deployExisting {
				fmt.Fprintln(io.Out, "App is not running, deploy...")
				ctx = app.WithName(ctx, cfg.AppName)
				ctx = appv2.WithName(ctx, cfg.AppName)
				return deploy.DeployWithConfig(ctx, cfg, deploy.DeployWithConfigArgs{
					ForceNomad:    flag.GetBool(ctx, "force-nomad"),
					ForceMachines: flag.GetBool(ctx, "force-machines"),
					ForceYes:      flag.GetBool(ctx, "now"),
				})
			}
		} else {
			fmt.Fprintln(io.Out, "An existing fly.toml file was found")
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
			if matched, _ := regexp.MatchString(`^[aeiou]`, strings.ToLower(srcInfo.Family)); matched {
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

	if !flag.GetBool(ctx, "generate-name") {
		if appName := flag.GetString(ctx, "name"); appName == "" {
			// Prompt the user for the app name
			if inputName, err := prompt.SelectAppName(ctx); err != nil {
				return err
			} else {
				appConfig.AppName = inputName
			}
		} else {
			appConfig.AppName = appName
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
		Name:            appConfig.AppName,
		OrganizationID:  org.ID,
		PreferredRegion: &region.Code,
	}

	createdApp, err := client.CreateApp(ctx, input)
	if err != nil {
		return err
	}
	if !importedConfig {
		appConfig.Definition = createdApp.Config.Definition
	}

	appConfig.AppName = createdApp.Name

	fmt.Fprintf(io.Out, "Created app %s in organization %s\n", createdApp.Name, org.Slug)

	adminLink := fmt.Sprintf("https://fly.io/apps/%s", createdApp.Name)
	appLink := fmt.Sprintf("%s.fly.dev", createdApp.Name)
	fmt.Fprintf(io.Out, "Admin URL: %s\nHostname: %s\n", adminLink, appLink)

	// If secrets are requested by the launch scanner, ask the user to input them
	if srcInfo != nil && len(srcInfo.Secrets) > 0 {
		secrets := make(map[string]string)
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
				surveyInput := &survey.Input{Message: prompt, Help: secret.Help}
				survey.AskOne(surveyInput, &val)
			}

			if val != "" {
				secrets[secret.Key] = val
			}
		}

		if len(secrets) > 0 {
			_, err := client.SetSecrets(ctx, appConfig.AppName, secrets)
			if err != nil {
				return err
			}
			fmt.Fprintf(io.Out, "Set secrets on %s: %s\n", appConfig.AppName, strings.Join(lo.Keys(secrets), ", "))
		}
	}

	// If volumes are requested by the launch scanner, create them
	if srcInfo != nil && len(srcInfo.Volumes) > 0 {
		for _, vol := range srcInfo.Volumes {
			appID, err := client.GetAppID(ctx, appConfig.AppName)
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
			LaunchPostgres(ctx, appConfig.AppName, org, region)
			options["postgresql"] = true
		}

		confirmRedis, err := prompt.Confirm(ctx, "Would you like to set up an Upstash Redis database now?")
		if confirmRedis && err == nil {
			LaunchRedis(ctx, appConfig.AppName, org, region)
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

		if srcInfo.HttpCheckPath != "" {
			appConfig.SetHttpCheck(srcInfo.HttpCheckPath)
		}

		if srcInfo.Concurrency != nil {
			appConfig.SetConcurrency(srcInfo.Concurrency["soft_limit"], srcInfo.Concurrency["hard_limit"])
		}

		for envName, envVal := range srcInfo.Env {
			if envVal == "APP_FQDN" {
				appConfig.SetEnvVariable(envName, appConfig.AppName+".fly.dev")
			} else {
				appConfig.SetEnvVariable(envName, envVal)
			}
		}

		if len(srcInfo.Statics) > 0 {
			var appStatics []app.Static
			for _, s := range srcInfo.Statics {
				appStatics = append(appStatics, app.Static{
					GuestPath: s.GuestPath,
					UrlPrefix: s.UrlPrefix,
				})
			}
			appConfig.SetStatics(appStatics)
		}

		if len(srcInfo.Volumes) > 0 {
			var appVolumes []app.Volume
			for _, v := range srcInfo.Volumes {
				appVolumes = append(appVolumes, app.Volume{
					Source:      v.Source,
					Destination: v.Destination,
				})
			}
			appConfig.SetVolumes(appVolumes)
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

		// Append any requested Dockerfile entries
		if len(srcInfo.DockerfileAppendix) > 0 {
			if err := appendDockerfileAppendix(srcInfo.DockerfileAppendix); err != nil {
				return fmt.Errorf("failed appending Dockerfile appendix: %w", err)
			}
		}

		if len(srcInfo.BuildArgs) > 0 {
			if appConfig.Build == nil {
				appConfig.Build = &app.Build{}
			}
			appConfig.Build.Args = srcInfo.BuildArgs
		}
	}

	if n := flag.GetInt(ctx, "internal-port"); n > 0 {
		appConfig.SetInternalPort(n)
	}

	// Finally, write the config
	if err := appConfig.WriteToDisk(ctx, configFilePath); err != nil {
		return err
	}

	ctx = app.WithConfig(ctx, appConfig)

	if srcInfo == nil {
		return nil
	}

	deployArgs := deploy.DeployWithConfigArgs{
		ForceNomad:    flag.GetBool(ctx, "force-nomad"),
		ForceMachines: flag.GetBool(ctx, "force-machines"),
		ForceYes:      flag.GetBool(ctx, "now"),
	}
	var v2AppConfig *appv2.Config
	if deployArgs.ForceMachines {
		v2AppConfig, err = appv2.LoadConfig(configFilePath)
		if err != nil {
			return fmt.Errorf("invalid config: %w", err)
		}
		ctx = appv2.WithConfig(ctx, v2AppConfig)
	}
	if deployArgs.ForceMachines && !deployArgs.ForceYes {
		if !flag.GetBool(ctx, "no-deploy") && !flag.GetBool(ctx, "now") && !flag.GetBool(ctx, "auto-confirm") && v2AppConfig.HasNonHttpAndHttpsStandardServices() {
			hasUdpService := v2AppConfig.HasUdpService()
			ipStuffStr := "a dedicated ipv4 address"
			if !hasUdpService {
				ipStuffStr = "dedicated ipv4 and ipv6 addresses"
			}
			confirmDedicatedIp, err := prompt.Confirmf(ctx, "Would you like to allocate %s now?", ipStuffStr)
			if confirmDedicatedIp && err == nil {
				v4Dedicated, err := client.AllocateIPAddress(ctx, v2AppConfig.AppName, "v4", "", nil, "")
				if err != nil {
					return err
				}
				fmt.Fprintf(io.Out, "Allocated dedicated ipv4: %s\n", v4Dedicated.Address)
				if !hasUdpService {
					v6Dedicated, err := client.AllocateIPAddress(ctx, v2AppConfig.AppName, "v6", "", nil, "")
					if err != nil {
						return err
					}
					fmt.Fprintf(io.Out, "Allocated dedicated ipv6: %s\n", v6Dedicated.Address)
				}
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
		return deploy.DeployWithConfig(ctx, appConfig, deployArgs)
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

func LaunchPostgres(ctx context.Context, appName string, org *api.Organization, region *api.Region) {
	io := iostreams.FromContext(ctx)
	clusterAppName := appName + "-db"
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
			AppName:   appName,
		})

		if err != nil {
			msg := `Failed attaching %s to the Postgres cluster %s: %w.\nTry attaching manually with 'fly postgres attach --app %s %s'`
			fmt.Fprintf(io.Out, msg, appName, clusterAppName, err, appName, clusterAppName)

		} else {
			fmt.Fprintf(io.Out, "Postgres cluster %s is now attached to %s\n", clusterAppName, appName)
		}
	}
}

func LaunchRedis(ctx context.Context, appName string, org *api.Organization, region *api.Region) {
	name := appName + "-redis"
	db, err := redis.Create(ctx, org, name, region, "", true, false)

	if err != nil {
		fmt.Println(fmt.Errorf("%w", err))
	} else {
		redis.AttachDatabase(ctx, db, appName)
	}
}
