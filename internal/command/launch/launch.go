package launch

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"

	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/sourcecode"
	"github.com/superfly/flyctl/pkg/flaps"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func New() (cmd *cobra.Command) {
	const (
		long  = `Create and configure a new app from source code or a Docker image.`
		short = long
	)

	cmd = command.New("launch", short, long, run, command.RequireSession, command.LoadAppConfigIfPresent)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.Region(),
		flag.Image(),
		flag.Now(),
		flag.RemoteOnly(true),
		flag.LocalOnly(),
		flag.BuildOnly(),
		flag.Push(),
		flag.Org(),
		flag.Dockerfile(),
		flag.Bool{
			Name:        "no-deploy",
			Description: "Do not prompt for deployment",
		},
		flag.Bool{
			Name:        "copy-config",
			Description: "Use the configuration file if present, without prompting",
		},
		flag.Bool{
			Name:        "generate-name",
			Description: "Always generate a name for the app, without prompting",
		},
		flag.String{
			Name:        "path",
			Description: `Path to the app source root, where fly.toml file will be saved`,
			Default:     ".",
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
	// Prompt for an app name
	var appName string

	if !flag.GetBool(ctx, "generate-name") {
		if appName, err = apps.SelectAppName(ctx); err != nil {
			return
		}
	}

	// Prompt for an org
	org, err := prompt.Org(ctx, nil)

	if err != nil {
		return
	}

	// If we potentially are deploying, launch a remote builder to prepare for deployment

	if !flag.GetBool(ctx, "no-deploy") {
		go imgsrc.EagerlyEnsureRemoteBuilder(ctx, client, org.Slug)
	}

	// Create the app

	input := api.CreateAppInput{
		Name:           appName,
		OrganizationID: org.ID,
	}

	mApp, err := client.CreateApp(ctx, input)

	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Created app %s in org %s\n", mApp.Name, org.Slug)

	// TODO: Handle imported fly.toml config

	// Setup new fly.toml config file

	appConfig := app.NewConfig()
	appConfig.AppName = mApp.Name

	// Launch in the specified region, or when not specified, in the nearest region
	regionCode := flag.GetString(ctx, "region")

	if regionCode == "" {
		region, err := client.GetNearestRegion(ctx)

		if err != nil {
			return err
		}

		regionCode = region.Code
	}

	appConfig.PrimaryRegion = regionCode

	// Determine whether to deploy from an image, a specified Dokerfile or scan the dir for a recognized project type
	if img := flag.GetString(ctx, "image"); img != "" {
		fmt.Fprintf(io.Out, "Lauching with image: %s", img)

		appConfig.Build = &app.Build{
			Image: img,
		}
	} else if dockerfile := flag.GetString(ctx, "dockerfile"); dockerfile != "" {
		fmt.Fprintf(io.Out, "Launching with Dockerfile: %s", dockerfile)

		appConfig.Build = &app.Build{
			Dockerfile: dockerfile,
		}
	} else {
		scanAndConfigure(ctx, workingDir, appConfig)
	}

	// If this project runs an http service, setup it up in fly.toml

	var httpService bool = false

	if appConfig.HttpService == nil {
		httpService, err = prompt.Confirm(ctx, "Does this app run an http service?")

		if err != nil {
			return
		}

		if httpService {
			err = setupHttpService(ctx, appConfig)
		}

		return
	}

	appConfig.WriteToDisk()

	fmt.Fprintf(io.Out, "Wrote to fly.toml\n")

	return deploy(ctx, appConfig)
}

func scanAndConfigure(ctx context.Context, dir string, appConfig *app.Config) (err error) {

	io := iostreams.FromContext(ctx)

	var srcInfo = new(sourcecode.SourceInfo)

	if scannedDirInfo, err := sourcecode.Scan(dir); err != nil {
		return err
	} else {
		srcInfo = scannedDirInfo
	}

	if srcInfo == nil {
		message := "Could not find a Dockerfile, nor detect a runtime or framework from source code. Continuing with a blank app."
		fmt.Fprint(io.Out, io.ColorScheme().Green(message))
		return err
	}

	// Tell the user which app type was detected
	printAppType(ctx, srcInfo)

	// Setup app config build section
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

	// Install files specified by
	err = installFiles(ctx, dir, srcInfo)

	if err != nil {
		return err
	}

	setScannerPrefs(ctx, appConfig, srcInfo)
	return
}

func setScannerPrefs(ctx context.Context, appConfig *app.Config, srcInfo *sourcecode.SourceInfo) (err error) {

	client := client.FromContext(ctx).API()

	if srcInfo.Port > 0 {
		appConfig.SetInternalPort(srcInfo.Port)
	}

	for envName, envVal := range srcInfo.Env {
		if envVal == "APP_FQDN" {
			appConfig.SetEnvVariable(envName, appConfig.AppName+".fly.dev")
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

	if len(srcInfo.Secrets) > 0 {
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
			_, err := client.SetSecrets(ctx, appConfig.AppName, secrets)

			if err != nil {
				return err
			}
			fmt.Printf("Set secrets on %s: %s\n", appConfig.AppName, strings.Join(keys, ", "))
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
				Region:    appConfig.PrimaryRegion,
				SizeGb:    1,
				Encrypted: true,
			})

			if err != nil {
				return err
			} else {
				fmt.Printf("Created a %dGB volume %s in the %s region\n", volume.SizeGb, volume.ID, appConfig.PrimaryRegion)
			}

		}
	}

	return
}

func installFiles(ctx context.Context, dir string, srcInfo *sourcecode.SourceInfo) (err error) {
	for _, f := range srcInfo.Files {
		path := filepath.Join(dir, f.Path)

		overwrite, err := prompt.ConfirmOverwrite(ctx, path)

		if err != nil {
			return err
		}

		if helpers.FileExists(path) && !overwrite {
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
	return
}

func printAppType(ctx context.Context, srcInfo *sourcecode.SourceInfo) {
	io := iostreams.FromContext(ctx)

	var article string = "a"
	matched, _ := regexp.MatchString(`^[aeiou]`, strings.ToLower(srcInfo.Family))

	if matched {
		article += "n"
	}

	appType := srcInfo.Family

	if srcInfo.Version != "" {
		appType = appType + " " + srcInfo.Version
	}

	fmt.Printf("Detected %s %s app\n", article, io.ColorScheme().Green(appType))
}

func setupHttpService(ctx context.Context, appConfig *app.Config) (err error) {

	var internalPort string

	err = prompt.String(ctx, &internalPort, "Which port does your service listen on?", "8080", true)

	if err != nil {
		return
	}

	appConfig.HttpService = new(app.HttpService)
	appConfig.HttpService.ForceHttps = true
	appConfig.HttpService.InternalPort = internalPort

	return
}

func deploy(ctx context.Context, config *app.Config) (err error) {

	client := client.FromContext(ctx).API()

	app, err := client.GetAppCompact(ctx, config.AppName)

	if err != nil {
		return
	}

	flapsClient, err := flaps.New(ctx, app)

	if err != nil {
		return
	}

	machineConfig := &api.MachineConfig{
		Image: config.Build.Image,
	}

	if config.HttpService != nil {
		machineConfig.Services = []interface{}{
			map[string]interface{}{
				"protocol":      "tcp",
				"internal_port": config.HttpService.InternalPort,
				"ports": []map[string]interface{}{
					{
						"port":     443,
						"handlers": []string{"http", "tls"},
					},
					{
						"port":        80,
						"handlers":    []string{"http"},
						"force_https": config.HttpService.ForceHttps,
					},
				},
			},
		}
	}
	err = config.Validate()

	if err != nil {
		return err
	}

	launchInput := api.LaunchMachineInput{
		AppID:   config.AppName,
		OrgSlug: app.Organization.ID,
		Region:  config.PrimaryRegion,
		Config:  machineConfig,
	}

	_, err = flapsClient.Launch(ctx, launchInput)

	return
}
