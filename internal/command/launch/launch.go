package launch

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cavaliergopher/grab/v3"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/scanner"
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
	existingConfig := appconfig.ConfigFromContext(ctx)
	generateName := flag.GetBool(ctx, "generate-name")
	copyConfig := flag.GetBool(ctx, "copy-config")
	name := strings.TrimSpace(flag.GetString(ctx, "name"))
	appConfig := appconfig.NewConfig()
	launchIntoExistingApp := false

	deployArgs := deploy.DeployWithConfigArgs{
		ForceNomad:    flag.GetBool(ctx, "force-nomad"),
		ForceMachines: flag.GetBool(ctx, "force-machines"),
		ForceYes:      flag.GetBool(ctx, "now"),
	}

	// Determine the working directory
	if absDir, err := filepath.Abs(workingDir); err == nil {
		workingDir = absDir
	}

	configFilePath := filepath.Join(workingDir, appconfig.DefaultConfigFileName)

	if existingConfig != nil {
		if existingConfig.AppName != "" {
			fmt.Fprintln(io.Out, "An existing fly.toml file was found for app", existingConfig.AppName)
		} else {
			fmt.Fprintln(io.Out, "An existing fly.toml file was found")
		}

		if !copyConfig {
			copyConfig, err = prompt.Confirm(ctx, "Would you like to copy its configuration to the new app?")
			if err != nil {
				return err
			}
		}

		if copyConfig {
			appConfig = existingConfig
		}
	}

	fmt.Fprintln(io.Out, "Creating app in", workingDir)

	srcInfo := new(scanner.SourceInfo)
	config := &scanner.ScannerConfig{
		ExistingPort: appConfig.InternalPort(),
	}

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
		appConfig.Build = &appconfig.Build{
			Image: img,
		}
	} else if dockerfile := flag.GetString(ctx, "dockerfile"); dockerfile != "" {
		if strings.HasPrefix(dockerfile, "http://") || strings.HasPrefix(dockerfile, "https://") {
			fmt.Fprintln(io.Out, "Downloading dockerfile", dockerfile)
			resp, err := grab.Get("Dockerfile", dockerfile)
			if err != nil {
				return err
			} else {
				appConfig.Build = &appconfig.Build{
					Dockerfile: resp.Filename,
				}

				// scan Dockerfile for port
				if si, err := scanner.Scan(workingDir, config); err != nil {
					return err
				} else {
					srcInfo = si
				}
			}
		} else {
			fmt.Fprintln(io.Out, "Using dockerfile", dockerfile)
			appConfig.Build = &appconfig.Build{
				Dockerfile: dockerfile,
			}
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

				appConfig.Build = &appconfig.Build{
					Builder:    srcInfo.Builder,
					Buildpacks: srcInfo.Buildpacks,
				}
			}
		}
	}

	if generateName {
		appConfig.AppName = ""
	}

	if name != "" {
		appConfig.AppName = name
	}

	if !generateName && name == "" {
		inputName, err := promptForAppName(ctx, appConfig)
		if err != nil {
			return err
		}
		appConfig.AppName = inputName
	}

	var org *api.Organization
	existingAppPlatform := ""

	if appConfig.AppName != "" {
		exists, app, err := appExists(ctx, appConfig)
		if err != nil {
			return err
		}

		if exists {
			msg := fmt.Sprintf("App %s already exists, do you want to launch into that app?", appConfig.AppName)
			launchIntoExistingApp, err = prompt.Confirm(ctx, msg)
			if err != nil {
				return err
			}
			if !launchIntoExistingApp {
				return nil
			}

			existingAppPlatform = app.PlatformVersion
			org = &api.Organization{
				ID:       app.Organization.ID,
				Name:     app.Organization.Name,
				Slug:     app.Organization.Slug,
				PaidPlan: app.Organization.PaidPlan,
			}
		}
	}

	// Prompt for an org
	if org == nil {
		org, err = prompt.Org(ctx)
		if err != nil {
			return
		}
	}

	// If we potentially are deploying, launch a remote builder to prepare for deployment.
	if !flag.GetBool(ctx, "no-deploy") {
		// TODO: determine if eager remote builder is still required here
		go imgsrc.EagerlyEnsureRemoteBuilder(ctx, client, org.Slug)
	}

	region, err := computeRegionToUse(ctx, appConfig, org.PaidPlan)
	if err != nil {
		return err
	}
	// Do not change PrimaryRegion after this line
	appConfig.PrimaryRegion = region.Code
	fmt.Fprintf(io.Out, "App will use '%s' region as primary\n", appConfig.PrimaryRegion)

	shouldUseMachines, err := shouldAppUseMachinesPlatform(ctx, org.Slug, existingAppPlatform)
	if err != nil {
		return err
	}

	if copyConfig && shouldUseMachines {
		// Check imported fly.toml is a valid V2 config before creating the app
		if err := appConfig.SetMachinesPlatform(); err != nil {
			return fmt.Errorf("Can not use configuration for Apps V2, check fly.toml: %w", err)
		}
	}

	switch {
	// Reuse app and local fly.toml
	case launchIntoExistingApp && copyConfig:
		// Placeholder
	case launchIntoExistingApp && !copyConfig:
		if !shouldUseMachines {
			return fmt.Errorf("Reusing the app but copying an existing fly.toml is only possible for V2 apps")
		}
		appConfig, err = freshV2Config(appConfig.AppName, appConfig)
		if err != nil {
			return err
		}
	// App doesn't exist, just create a new app
	case !launchIntoExistingApp:
		createdApp, err := client.CreateApp(ctx, api.CreateAppInput{
			Name:            appConfig.AppName,
			OrganizationID:  org.ID,
			PreferredRegion: &appConfig.PrimaryRegion,
			Machines:        shouldUseMachines,
		})
		if err != nil {
			return err
		}

		switch {
		case copyConfig:
			appConfig.AppName = createdApp.Name
		case shouldUseMachines:
			appConfig, err = freshV2Config(createdApp.Name, appConfig)
			if err != nil {
				return fmt.Errorf("failed to create new V2 app configuration: %w", err)
			}
		default:
			// Use the default configuration template suggested by Web
			appConfig, err = freshV1Config(createdApp.Name, appConfig, &createdApp.Config.Definition)
			if err != nil {
				return fmt.Errorf("failed to get new configuration: %w", err)
			}
		}
		fmt.Fprintf(io.Out, "Created app '%s' in organization '%s'\n", appConfig.AppName, org.Slug)
	}

	fmt.Fprintf(io.Out, "Admin URL: https://fly.io/apps/%s\n", appConfig.AppName)
	fmt.Fprintf(io.Out, "Hostname: %s.fly.dev\n", appConfig.AppName)

	// If files are requested by the launch scanner, create them.
	if err := createSourceInfoFiles(ctx, srcInfo, workingDir); err != nil {
		return err
	}
	// If secrets are requested by the launch scanner, ask the user to input them
	if err := createSecrets(ctx, srcInfo, appConfig.AppName); err != nil {
		return err
	}
	// If volumes are requested by the launch scanner, create them
	if err := createVolumes(ctx, srcInfo, appConfig.AppName, appConfig.PrimaryRegion); err != nil {
		return err
	}
	// If database are requested by the launch scanner, create them
	options, err := createDatabases(ctx, srcInfo, appConfig.AppName, region, org)
	if err != nil {
		return err
	}
	// Invoke Callback, if any
	if err := runCallback(ctx, srcInfo, options); err != nil {
		return err
	}
	// Run any initialization commands
	if err := runInitCommands(ctx, srcInfo); err != nil {
		return err
	}
	// Complete the appConfig
	if err := setAppconfigFromSrcinfo(ctx, srcInfo, appConfig); err != nil {
		return err
	}

	// Attempt to create a .dockerignore from .gitignore
	determineDockerIgnore(ctx, workingDir)

	// Override internal port if requested using --internal-port flag
	if n := flag.GetInt(ctx, "internal-port"); n > 0 {
		appConfig.SetInternalPort(n)
	}

	// Finally write application configuration to fly.toml
	if err := appConfig.WriteToDisk(ctx, configFilePath); err != nil {
		return err
	}

	if srcInfo == nil {
		return nil
	}

	ctx = appconfig.WithName(ctx, appConfig.AppName)
	ctx = appconfig.WithConfig(ctx, appConfig)

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

func shouldAppUseMachinesPlatform(ctx context.Context, orgSlug, existingAppPlatform string) (bool, error) {
	// Keep if we are reusing an app and it has platform version set
	if existingAppPlatform != "" {
		return existingAppPlatform == appconfig.MachinesPlatform, nil
	}
	// Otherwise looks for CLI flags and organization defaults
	apiClient := client.FromContext(ctx).API()
	if flag.GetBool(ctx, "force-machines") {
		return true, nil
	} else if flag.GetBool(ctx, "force-nomad") {
		return false, nil
	}
	// Query the organization looking for default platform version to use
	orgDefault, err := apiClient.GetAppsV2DefaultOnForOrg(ctx, orgSlug)
	if err != nil {
		return false, err
	}
	return orgDefault, nil
}

func appExists(ctx context.Context, cfg *appconfig.Config) (bool, *api.AppBasic, error) {
	client := client.FromContext(ctx).API()
	app, err := client.GetAppBasic(ctx, cfg.AppName)
	if err != nil {
		if api.IsNotFoundError(err) || graphql.IsNotFoundError(err) {
			return false, nil, nil
		}
		return false, nil, err
	}

	return true, app, nil
}

func promptForAppName(ctx context.Context, cfg *appconfig.Config) (name string, err error) {
	if cfg.AppName == "" {
		return prompt.SelectAppName(ctx)
	}

	msg := fmt.Sprintf("Choose an app name (leaving blank will default to '%s')", cfg.AppName)
	name, err = prompt.SelectAppNameWithMsg(ctx, msg)
	if err != nil {
		return name, err
	}

	// default to cfg.name if user doesn't enter any name after copying the configuration
	if name == "" {
		name = cfg.AppName
	}

	return
}
