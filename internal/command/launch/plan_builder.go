package launch

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/haikunator"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/scanner"
	"github.com/superfly/graphql"
)

// Cache values between buildManifest and stateFromManifest
// It's important that we feed the result of buildManifest into stateFromManifest,
// because that prevents the launch-manifest -> edit/save -> launch-from-manifest
// path from going out of sync with the standard launch path.
// Doing this can lead to double-calculation, especially of scanners which could
// have a lot of processing to do. Hence, a cache :)
type planBuildCache struct {
	appConfig *appconfig.Config
	srcInfo   *scanner.SourceInfo
}

func buildManifest(ctx context.Context) (*LaunchManifest, *planBuildCache, error) {

	appConfig, copiedConfig, err := determineBaseAppConfig(ctx)
	if err != nil {
		return nil, nil, err
	}

	// TODO(allison): possibly add some automatic suffixing to app names if they already exist

	org, orgExplanation, err := determineOrg(ctx)
	if err != nil {
		return nil, nil, err
	}

	region, regionExplanation, err := determineRegion(ctx, appConfig, org.PaidPlan)
	if err != nil {
		return nil, nil, err
	}

	if copiedConfig {
		// Check imported fly.toml is a valid V2 config before creating the app
		if err := appConfig.SetMachinesPlatform(); err != nil {
			return nil, nil, fmt.Errorf("can not use configuration for Fly Launch, check fly.toml: %w", err)
		}
	}

	workingDir := flag.GetString(ctx, "path")
	if absDir, err := filepath.Abs(workingDir); err == nil {
		workingDir = absDir
	}
	configPath := filepath.Join(workingDir, appconfig.DefaultConfigFileName)

	var srcInfo *scanner.SourceInfo
	srcInfo, appConfig.Build, err = determineSourceInfo(ctx, appConfig, copiedConfig, workingDir)
	if err != nil {
		return nil, nil, err
	}

	appName, appNameExplanation, err := determineAppName(ctx, configPath)
	if err != nil {
		return nil, nil, err
	}

	guest, guestExplanation, err := determineGuest(ctx, appConfig, srcInfo)
	if err != nil {
		return nil, nil, err
	}

	// TODO: Determine databases requested by the sourceInfo, and add them to the plan.

	lp := &plan.LaunchPlan{
		AppName:          appName,
		OrgSlug:          org.Slug,
		RegionCode:       region.Code,
		HighAvailability: flag.GetBool(ctx, "ha"),
		CPUKind:          guest.CPUKind,
		CPUs:             guest.CPUs,
		MemoryMB:         guest.MemoryMB,
		VmSize:           guest.ToSize(),
		HttpServicePort:  8080,
		Postgres:         plan.PostgresPlan{},
		Redis:            plan.RedisPlan{},
	}

	planSource := &launchPlanSource{
		appNameSource:  appNameExplanation,
		regionSource:   regionExplanation,
		orgSource:      orgExplanation,
		guestSource:    guestExplanation,
		postgresSource: "not requested",
		redisSource:    "not requested",
	}

	if srcInfo != nil {
		lp.ScannerFamily = srcInfo.Family
		const scannerSource = "determined from app source"
		switch srcInfo.DatabaseDesired {
		case scanner.DatabaseKindPostgres:
			lp.Postgres = plan.DefaultPostgres(lp)
			planSource.postgresSource = scannerSource
		case scanner.DatabaseKindMySQL:
			// TODO
		case scanner.DatabaseKindSqlite:
			// TODO
		}
		if srcInfo.RedisDesired {
			lp.Redis = plan.DefaultRedis(lp)
			planSource.redisSource = scannerSource
		}
		if srcInfo.Port != 0 {
			lp.HttpServicePort = srcInfo.Port
		}
	}

	return &LaunchManifest{
			Plan:       lp,
			PlanSource: planSource,
		}, &planBuildCache{
			appConfig: appConfig,
			srcInfo:   srcInfo,
		}, nil

}

func stateFromManifest(ctx context.Context, m LaunchManifest, optionalCache *planBuildCache) (*launchState, error) {

	var (
		io        = iostreams.FromContext(ctx)
		client    = client.FromContext(ctx)
		clientApi = client.API()
	)

	org, err := clientApi.GetOrganizationBySlug(ctx, m.Plan.OrgSlug)
	if err != nil {
		return nil, err
	}

	// If we potentially are deploying, launch a remote builder to prepare for deployment.
	if !flag.GetBool(ctx, "no-deploy") {
		// TODO: determine if eager remote builder is still required here
		go imgsrc.EagerlyEnsureRemoteBuilder(ctx, clientApi, org.Slug)
	}

	var (
		appConfig    *appconfig.Config
		copiedConfig bool
	)
	if optionalCache != nil {
		appConfig = optionalCache.appConfig
	} else {
		appConfig, copiedConfig, err = determineBaseAppConfig(ctx)
		if err != nil {
			return nil, err
		}

		if copiedConfig {
			// Check imported fly.toml is a valid V2 config before creating the app
			if err := appConfig.SetMachinesPlatform(); err != nil {
				return nil, fmt.Errorf("can not use configuration for Fly Launch, check fly.toml: %w", err)
			}
		}
	}

	var envVars map[string]string
	envFlags := flag.GetStringArray(ctx, "env")
	if len(envFlags) > 0 {
		envVars, err = cmdutil.ParseKVStringsToMap(envFlags)
		if err != nil {
			return nil, fmt.Errorf("failed parsing --env flags: %w", err)
		}
	}

	if taken, _ := appNameTaken(ctx, appConfig.AppName); taken {
		return nil, fmt.Errorf("app name %s is already taken", appConfig.AppName)
	}

	workingDir := flag.GetString(ctx, "path")
	if absDir, err := filepath.Abs(workingDir); err == nil {
		workingDir = absDir
	}
	configPath := filepath.Join(workingDir, appconfig.DefaultConfigFileName)
	fmt.Fprintln(io.Out, "Creating app in", workingDir)

	var srcInfo *scanner.SourceInfo

	if optionalCache != nil {
		srcInfo = optionalCache.srcInfo
	} else {
		srcInfo, appConfig.Build, err = determineSourceInfo(ctx, appConfig, copiedConfig, workingDir)
		if err != nil {
			return nil, err
		}

		scannerFamily := ""
		if srcInfo != nil {
			scannerFamily = srcInfo.Family
		}
		if m.Plan.ScannerFamily != scannerFamily {
			got := familyToAppType(scannerFamily)
			expected := familyToAppType(m.Plan.ScannerFamily)
			return nil, fmt.Errorf("launch manifest was created for a %s, but this is a %s", expected, got)
		}
	}

	return &launchState{
		workingDir: workingDir,
		configPath: configPath,
		LaunchManifest: LaunchManifest{
			m.Plan,
			m.PlanSource,
		},
		env:        envVars,
		appConfig:  appConfig,
		sourceInfo: srcInfo,
		cache:      map[string]interface{}{},
	}, nil
}

// determineBaseAppConfig looks for existing app config, ask to reuse or returns an empty config
// TODO(allison): remove the prompt once we determine the proper default behavior
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
			case prompt.IsNonInteractive(err) && !flag.GetYes(ctx):
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
	if err := newCfg.SetMachinesPlatform(); err != nil {
		return nil, false, err
	}

	return newCfg, false, nil
}

// determineAppName determines the app name from the config file or directory name
func determineAppName(ctx context.Context, configPath string) (string, string, error) {

	appName := flag.GetString(ctx, "name")
	if appName == "" {
		appName = filepath.Base(filepath.Dir(configPath))
	}
	if appName == "" {
		return "", "", errors.New("enable to determine app name, please specify one with --name")
	}
	// If the app name is already taken, try to generate a unique suffix.
	if taken, _ := appNameTaken(ctx, appName); taken {
		delimiter := "-"
		var newName string
		found := false
		for i := 1; i < 10; i++ {
			newName = fmt.Sprintf("%s%s%s", appName, delimiter, haikunator.Haikunator().Delimiter(delimiter))
			if taken, _ := appNameTaken(ctx, newName); !taken {
				found = true
				break
			}
		}
		if !found {
			// TODO: Use FlyErr
			return "", "", fmt.Errorf("unable to find an available app name for %s", appName)
		}
		appName = newName
	}
	return appName, "derived from your directory name", nil
}

func appNameTaken(ctx context.Context, name string) (bool, error) {
	client := client.FromContext(ctx).API()
	_, err := client.GetAppBasic(ctx, name)
	if err != nil {
		if api.IsNotFoundError(err) || graphql.IsNotFoundError(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// determineOrg returns the org specified on the command line, or the personal org if left unspecified
func determineOrg(ctx context.Context) (*api.Organization, string, error) {
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

// determineRegion returns the region to use for a new app. In order, it tries:
//  1. the primary_region field of the config, if one exists
//  2. the region specified on the command line, if specified
//  3. the nearest region to the user
func determineRegion(ctx context.Context, config *appconfig.Config, paidPlan bool) (*api.Region, string, error) {

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

// determineGuest returns the guest type to use for a new app.
// Currently, it defaults to shared-cpu-1x
func determineGuest(ctx context.Context, config *appconfig.Config, srcInfo *scanner.SourceInfo) (*api.MachineGuest, string, error) {
	def := api.MachinePresets["shared-cpu-1x"]
	reason := "most apps need about 1GB of RAM"

	guest, err := flag.GetMachineGuest(ctx, helpers.Clone(def))
	if err != nil {
		return nil, reason, err
	}

	if def.CPUs != guest.CPUs || def.CPUKind != guest.CPUKind || def.MemoryMB != guest.MemoryMB {
		reason = "specified on the command line"
	}
	return guest, reason, nil
}
