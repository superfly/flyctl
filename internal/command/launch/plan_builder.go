package launch

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/scanner"
)

// v2BuildPlan creates a launchState from command line flags.
// It shouldn't have any filesystem side effects.
func v2BuildPlan(ctx context.Context) (*launchState, error) {

	var (
		io        = iostreams.FromContext(ctx)
		client    = client.FromContext(ctx)
		clientApi = client.API()
	)

	appConfig, copiedConfig, err := v2DetermineBaseAppConfig(ctx)
	if err != nil {
		return nil, err
	}

	// TODO(allison): possibly add some automatic suffixing to app names if they already exist

	org, orgExplanation, err := v2DetermineOrg(ctx)
	if err != nil {
		return nil, err
	}

	// If we potentially are deploying, launch a remote builder to prepare for deployment.
	if !flag.GetBool(ctx, "no-deploy") {
		// TODO: determine if eager remote builder is still required here
		go imgsrc.EagerlyEnsureRemoteBuilder(ctx, clientApi, org.Slug)
	}

	region, regionExplanation, err := v2DetermineRegion(ctx, appConfig, org.PaidPlan)
	if err != nil {
		return nil, err
	}

	var envVars map[string]string = nil
	envFlags := flag.GetStringArray(ctx, "env")
	if len(envFlags) > 0 {
		envVars, err = cmdutil.ParseKVStringsToMap(envFlags)
		if err != nil {
			return nil, fmt.Errorf("failed parsing --env flags: %w", err)
		}
	}

	if copiedConfig {
		// Check imported fly.toml is a valid V2 config before creating the app
		if err := appConfig.SetMachinesPlatform(); err != nil {
			return nil, fmt.Errorf("can not use configuration for Fly Launch, check fly.toml: %w", err)
		}
	}

	workingDir := flag.GetString(ctx, "path")
	if absDir, err := filepath.Abs(workingDir); err == nil {
		workingDir = absDir
	}
	configPath := filepath.Join(workingDir, appconfig.DefaultConfigFileName)
	fmt.Fprintln(io.Out, "Creating app in", workingDir)

	var srcInfo *scanner.SourceInfo
	srcInfo, appConfig.Build, err = determineSourceInfo(ctx, appConfig, copiedConfig, workingDir)
	if err != nil {
		return nil, err
	}

	appName, appNameExplanation, err := v2DetermineAppName(ctx, configPath)
	if err != nil {
		return nil, err
	}

	guest, guestExplanation, err := v2DetermineGuest(ctx, appConfig, srcInfo)
	if err != nil {
		return nil, err
	}

	// TODO: Determine databases requested by the sourceInfo, and add them to the plan.

	lp := &launchPlan{
		AppName:        appName,
		appNameSource:  appNameExplanation,
		Region:         region,
		regionSource:   regionExplanation,
		Org:            org,
		orgSource:      orgExplanation,
		Guest:          guest,
		guestSource:    guestExplanation,
		Postgres:       nil,
		postgresSource: "not implemented",
		Redis:          nil,
		redisSource:    "not implemented",
		Env:            envVars,
	}

	return &launchState{
		workingDir: workingDir,
		configPath: configPath,
		plan:       lp,
		appConfig:  appConfig,
		sourceInfo: srcInfo,
	}, nil
}

// determineBaseAppConfig looks for existing app config, ask to reuse or returns an empty config
// TODO(allison): remove the prompt once we determine the proper default behavior
func v2DetermineBaseAppConfig(ctx context.Context) (*appconfig.Config, bool, error) {
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
			case prompt.IsNonInteractive(err) && !flag.GetBool(ctx, "auto-confirm"):
				return nil, false, err
			case err != nil:
				return nil, false, err
			}
		}

		if copyConfig {
			return existingConfig, true, nil
		}
	}

	newCfg := appconfig.NewConfig()
	newCfg.HTTPService = &appconfig.HTTPService{
		InternalPort:       8080,
		ForceHTTPS:         true,
		AutoStartMachines:  api.Pointer(true),
		AutoStopMachines:   api.Pointer(true),
		MinMachinesRunning: api.Pointer(0),
		Processes:          []string{"app"},
	}
	if err := newCfg.SetMachinesPlatform(); err != nil {
		return nil, false, err
	}

	return newCfg, false, nil
}

// v2DetermineAppName determines the app name from the config file or directory name
func v2DetermineAppName(ctx context.Context, configPath string) (string, string, error) {

	appName := flag.GetString(ctx, "name")
	if appName == "" {
		appName = filepath.Base(filepath.Dir(configPath))
	}
	if appName == "" {
		return "", "", errors.New("enable to determine app name, please specify one with --name")
	}
	return appName, "derived from your directory name", nil
}

// v2DetermineOrg returns the org specified on the command line, or the personal org if left unspecified
func v2DetermineOrg(ctx context.Context) (*api.Organization, string, error) {
	var (
		client    = client.FromContext(ctx)
		clientApi = client.API()
	)

	personal, others, err := clientApi.GetCurrentOrganizations(ctx)
	if err != nil {
		return nil, "", err
	}

	orgSlug := flag.GetOrg(ctx)
	if orgSlug == "" {
		return &personal, "fly launch defaults to the personal org", nil
	}

	org, found := lo.Find(others, func(o api.Organization) bool {
		return o.Slug == orgSlug
	})
	if !found {
		return nil, "", fmt.Errorf("organization '%s' not found", orgSlug)
	}
	return &org, "specified on the command line", nil
}

// v2DetermineRegion returns the region to use for a new app. In order, it tries:
//  1. the primary_region field of the config, if one exists
//  2. the region specified on the command line, if specified
//  3. the nearest region to the user
func v2DetermineRegion(ctx context.Context, config *appconfig.Config, paidPlan bool) (*api.Region, string, error) {

	client := client.FromContext(ctx)
	regionCode := flag.GetRegion(ctx)
	explanation := "specified on the command line"

	if regionCode == "" {
		regionCode = config.PrimaryRegion
		explanation = "from your fly.toml"
	}

	if regionCode != "" {
		region, err := getRegionByCode(ctx, regionCode)
		return region, explanation, err
	}

	// Get the closest region
	// TODO(allison): does this return paid regions for free orgs?
	region, err := client.API().GetNearestRegion(ctx)
	return region, "this is the fastest region for you", err
}

// getRegionByCode returns the region with the IATA code, or an error if it doesn't exist
func getRegionByCode(ctx context.Context, regionCode string) (*api.Region, error) {
	apiClient := client.FromContext(ctx).API()

	allRegions, _, err := apiClient.PlatformRegions(ctx)
	if err != nil {
		return nil, err
	}

	for _, r := range allRegions {
		if r.Code == regionCode {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("Unknown region '%s'. Run `fly platform regions` to see valid names", regionCode)
}

// v2DetermineGuest returns the guest type to use for a new app.
// Currently, it defaults to shared-cpu-1x
func v2DetermineGuest(ctx context.Context, config *appconfig.Config, srcInfo *scanner.SourceInfo) (*api.MachineGuest, string, error) {
	shared1x := api.MachinePresets["shared-cpu-1x"]
	return shared1x, "most apps need about 1GB of RAM", nil
}
