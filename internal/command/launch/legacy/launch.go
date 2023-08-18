package legacy

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/logrusorgru/aurora"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/scanner"
	"github.com/superfly/graphql"
	"golang.org/x/exp/slices"
)

func Run(ctx context.Context) (err error) {

	io := iostreams.FromContext(ctx)
	client := client.FromContext(ctx).API()
	workingDir := flag.GetString(ctx, "path")

	// Note: this also disables --force-nomad when launching into an existing nomad app.
	// we're fast-tracking the removal of nomad support, so this should be fine.
	if flag.GetBool(ctx, "force-nomad") && !buildinfo.IsDev() {
		return fmt.Errorf("creating new apps on the nomad platform is no longer supported")
	}

	metrics.Started(ctx, "launch")
	defer func() {
		metrics.Status(ctx, "launch", err == nil)
	}()

	// Determine the working directory
	if absDir, err := filepath.Abs(workingDir); err == nil {
		workingDir = absDir
	}
	configFilePath := filepath.Join(workingDir, appconfig.DefaultConfigFileName)
	fmt.Fprintln(io.Out, "Creating app in", workingDir)

	appConfig, copyConfig, err := determineBaseAppConfig(ctx)
	if err != nil {
		return err
	}

	var srcInfo *scanner.SourceInfo
	srcInfo, appConfig.Build, err = determineSourceInfo(ctx, appConfig, copyConfig, workingDir)
	if err != nil {
		return err
	}

	appConfig.AppName, err = determineAppName(ctx, appConfig)
	if err != nil {
		return err
	}

	var org *api.Organization
	var existingAppPlatform string
	var launchIntoExistingApp bool

	if appConfig.AppName != "" {
		exists, app, err := appExists(ctx, appConfig)
		if err != nil {
			return err
		}

		if exists {
			launchIntoExistingApp = flag.GetBool(ctx, "reuse-app")
			if !flag.IsSpecified(ctx, "reuse-app") {
				msg := fmt.Sprintf("App %s already exists, do you want to launch into that app?", appConfig.AppName)
				launchIntoExistingApp, err = prompt.Confirm(ctx, msg)
				if err != nil {
					return err
				}
				if !launchIntoExistingApp {
					return nil
				}
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
	fmt.Fprintf(io.Out, "App will use '%s' region as primary\n\n", appConfig.PrimaryRegion)

	shouldUseMachines, err := shouldAppUseMachinesPlatform(ctx, org.Slug, existingAppPlatform)
	if err != nil {
		return err
	}

	using_appsv1_only_feature := false
	if s := flag.GetString(ctx, "strategy"); s != "" && !slices.Contains(appconfig.MachinesDeployStrategies, s) {
		using_appsv1_only_feature = true
	}

	if !shouldUseMachines && !using_appsv1_only_feature {
		fmt.Fprintf(io.ErrOut, "%s Apps v1 Platform is deprecated. We recommend using the --force-machines flag, or setting\nyour organization's default for new apps to Apps v2 with 'fly orgs apps-v2 default-on <org-name>'\n", aurora.Yellow("WARN"))
	}

	var envVars map[string]string = nil
	envFlags := flag.GetStringArray(ctx, "env")
	if len(envFlags) > 0 {
		envVars, err = cmdutil.ParseKVStringsToMap(envFlags)
		if err != nil {
			return errors.Wrap(err, "parsing --env flags")
		}
	}

	if copyConfig && shouldUseMachines {
		// Check imported fly.toml is a valid V2 config before creating the app
		if err := appConfig.SetMachinesPlatform(); err != nil {
			return fmt.Errorf("Can not use configuration for Apps V2, check fly.toml: %w", err)
		}
	}

	switch {
	// App exists and it is not importing existing fly.toml
	case launchIntoExistingApp && !copyConfig:
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

	if envVars != nil {
		appConfig.SetEnvVariables(envVars)
	}
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
	if err := runCallback(ctx, appConfig.AppName, srcInfo, options); err != nil {
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
	appConfig.SetConfigFilePath(configFilePath)
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

		// Reload and validate the app config in case the user edited it before confirming
		if deployNow {
			path := appConfig.ConfigFilePath()
			newCfg, err := appconfig.LoadConfig(path)
			if err != nil {
				return fmt.Errorf("failed to reload configuration file %s: %w", path, err)
			}

			if appConfig.ForMachines() {
				if err := newCfg.SetMachinesPlatform(); err != nil {
					return fmt.Errorf("failed to parse fly.toml, check the configuration format: %w", err)
				}
			}
			appConfig = newCfg
		}
	}

	err, extraInfo := appConfig.Validate(ctx)
	if extraInfo != "" {
		fmt.Fprintf(io.ErrOut, extraInfo)
	}
	if err != nil {
		return fmt.Errorf("invalid configuration file: %w", err)
	}

	if deployNow {
		return deploy.DeployWithConfig(ctx, appConfig, flag.GetBool(ctx, "now"), nil)
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

	forceMachines := flag.GetBool(ctx, "force-machines")
	forceNomad := flag.GetBool(ctx, "force-nomad")

	// Keep if we are reusing an app and it has platform version set
	if existingAppPlatform != "" {
		isV2 := existingAppPlatform == appconfig.MachinesPlatform
		switch {
		case forceMachines && !isV2:
			return false, fmt.Errorf("--force-machines won't work for existing app in nomad platform")
		case forceNomad && isV2:
			return false, fmt.Errorf("--force-nomad won't work for existing app in machines platform")
		default:
			return isV2, nil
		}
	}

	// Otherwise looks for CLI flags and organization defaults
	if forceMachines {
		return true, nil
	} else if forceNomad {
		return false, nil
	}

	// Default to Apps v2
	return true, nil
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

// determineBaseAppConfig looks for existing app config, ask to reuse or returns an empty config
func determineBaseAppConfig(ctx context.Context) (*appconfig.Config, bool, error) {
	io := iostreams.FromContext(ctx)

	existingConfig := appconfig.ConfigFromContext(ctx)
	if existingConfig != nil {

		if existingConfig.AppName != "" {
			fmt.Fprintln(io.Out, "An existing fly.toml file was found for app", existingConfig.AppName)
		} else {
			fmt.Fprintln(io.Out, "An existing fly.toml file was found")
		}

		copyConfig := flag.GetBool(ctx, "copy-config")
		if !flag.IsSpecified(ctx, "copy-config") {
			var err error
			copyConfig, err = prompt.Confirm(ctx, "Would you like to copy its configuration to the new app?")
			switch {
			case prompt.IsNonInteractive(err) && !flag.HasYes(ctx):
				return nil, false, err
			case err != nil:
				return nil, false, err
			}
		}

		if copyConfig {
			return existingConfig, true, nil
		}
	}

	return appconfig.NewConfig(), false, nil
}

func determineAppName(ctx context.Context, appConfig *appconfig.Config) (string, error) {
	generateName := flag.GetBool(ctx, "generate-name")
	if generateName {
		return "", nil
	}

	name := strings.TrimSpace(flag.GetString(ctx, "name"))
	if name != "" {
		return name, nil
	}

	switch name, err := promptForAppName(ctx, appConfig); {
	case prompt.IsNonInteractive(err):
		return "", fmt.Errorf("--name or --generate-name flags must be specified when not running interactively")
	case err != nil:
		return "", err
	default:
		return name, nil
	}
}
