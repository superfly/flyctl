package launch

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
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"

	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/filemu"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/sourcecode"
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
		flag.ImageLabel(),
		flag.NoCache(),
		flag.BuildSecret(),
		flag.BuildArg(),
		flag.BuildTarget(),
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
		flag.Bool{
			Name:        "machines",
			Shorthand:   "m",
			Description: `Launch and deploy using the Machines API`,
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
	if flag.GetBool(ctx, "machines") {
		input.Machines = true
	}

	createdApp, err := client.CreateApp(ctx, input)

	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Created app %s in org %s\n", createdApp.Name, org.Slug)

	// TODO: Handle imported fly.toml config

	// Setup new fly.toml config file

	appConfig := app.NewConfig()

	// Config version 2 is for machine apps
	appConfig.Version = app.MachinesVersion
	appConfig.AppName = createdApp.Name

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

	var srcInfo *sourcecode.SourceInfo

	// Determine whether to deploy from an image
	if img := flag.GetString(ctx, "image"); img != "" {
		fmt.Fprintf(io.Out, "Lauching with image: %s", img)

		appConfig.Build.Image = img

		// Deploy from specified Dokerfile
	} else if dockerfile := flag.GetString(ctx, "dockerfile"); dockerfile != "" {
		fmt.Fprintf(io.Out, "Launching with Dockerfile: %s", dockerfile)

		appConfig.Build.Dockerfile = dockerfile

		// Scan the working directory for a compatible launcher
	} else {

		srcInfo, err = scanAndConfigure(ctx, workingDir, appConfig)

		if err != nil {
			return err
		}
	}

	// If this project runs an http service, setup it up in fly.toml
	var choseHttpService bool = false

	if appConfig.HttpService == nil {
		choseHttpService, err = prompt.Confirm(ctx, "Does this app run an HTTP service?")

		if err != nil {
			return
		}

		if choseHttpService {
			err = setupHttpService(ctx, appConfig)

			if err != nil {
				return err
			}
		}
		// Allocate an IP because machine apps don't do this automaticallt yet
		_, err = client.AllocateIPAddress(ctx, appConfig.AppName, "v4", "")

		if err != nil {
			fmt.Fprintln(io.Out, "Failed allocating IpV4 address")
		}

		_, err = client.AllocateIPAddress(ctx, appConfig.AppName, "v6", "")

		if err != nil {
			fmt.Fprintln(io.Out, "Failed allocating IpV6 address")
		}
	}

	appConfig.WriteToDisk()

	fmt.Fprintf(io.Out, "Wrote to fly.toml\n")

	var deployNow bool = false

	if !flag.GetBool(ctx, "no-deploy") && !srcInfo.SkipDeploy {
		if flag.GetBool(ctx, "now") {
			deployNow = true
		} else {
			deployNow, err = prompt.Confirm(ctx, "Would you like to deploy now?")
		}

		if deployNow {
			return deploy.DeployWithConfig(ctx, appConfig)
		}
	}

	// Alternative deploy documentation if our standard deploy method is not correct
	if srcInfo.DeployDocs != "" {
		fmt.Fprintln(io.Out, srcInfo.DeployDocs)
	} else {
		fmt.Fprintln(io.Out, "Your app is ready. Deploy with `flyctl deploy`")
	}

	return
}

func scanAndConfigure(ctx context.Context, dir string, appConfig *app.Config) (srcInfo *sourcecode.SourceInfo, err error) {

	io := iostreams.FromContext(ctx)

	srcInfo = new(sourcecode.SourceInfo)

	scannedDirInfo, err := sourcecode.Scan(dir)

	if err != nil {
		return srcInfo, err
	} else {
		srcInfo = scannedDirInfo
	}

	if srcInfo == nil {
		message := "We looked for a Dockerfile, supported runtime or supported framework, but didn't find any. So we'll start with a basic application."
		fmt.Fprint(io.Out, io.ColorScheme().Green(message))
		return srcInfo, err
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

		appConfig.Build.Builder = srcInfo.Builder
		appConfig.Build.Buildpacks = srcInfo.Buildpacks
	}

	// Install files specified by
	err = installFiles(ctx, dir, srcInfo)

	if err != nil {
		return srcInfo, err
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
	if len(srcInfo.Volumes) > 0 {

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

	// Set Docker build arguments
	if len(srcInfo.BuildArgs) > 0 {
		appConfig.Build.Args = srcInfo.BuildArgs
	}

	// Display notices to users
	if srcInfo.Notice != "" {
		fmt.Println(srcInfo.Notice)
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
