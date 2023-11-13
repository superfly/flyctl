package launch

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/haikunator"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/scanner"
	"github.com/superfly/graphql"
)

type recoverableInUiError struct {
	base error
}

func (e recoverableInUiError) String() string {
	var flyErr flyerr.GenericErr
	if errors.As(e.base, &flyErr) {
		if flyErr.Descript != "" {
			return fmt.Sprintf("%s\n%s\n", flyErr.Err, flyErr.Descript)
		}
	}
	return e.base.Error()
}
func (e recoverableInUiError) Error() string {
	return e.base.Error()
}
func (e recoverableInUiError) Unwrap() error {
	return e.base
}

// state.go uses this as a sentinel value to indicate that a value was not specified,
// and should therefore be displayed as <unspecified> in the plan display.
// I'd like to move off this in the future, but this is the quick 'n dirty initial path
const recoverableSpecifyInUi = "must be specified in UI"

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

func appNameTakenErr(appName string) error {
	return flyerr.GenericErr{
		Err:      fmt.Sprintf("app name %s is already taken", appName),
		Descript: "each Fly.io app must have a unique name",
		Suggest:  "Please specify a different app name with --name",
	}
}

func buildManifest(ctx context.Context, canEnterUi bool) (*LaunchManifest, *planBuildCache, error) {
	var recoverableInUiErrors []recoverableInUiError
	tryRecoverErr := func(e error) error {
		var asRecoverableErr recoverableInUiError
		if errors.As(e, &asRecoverableErr) && canEnterUi {
			recoverableInUiErrors = append(recoverableInUiErrors, asRecoverableErr)
			return nil
		}
		return e
	}

	appConfig, copiedConfig, err := determineBaseAppConfig(ctx)
	if err != nil {
		return nil, nil, err
	}

	// TODO(allison): possibly add some automatic suffixing to app names if they already exist

	org, orgExplanation, err := determineOrg(ctx)
	if err != nil {
		if err := tryRecoverErr(err); err != nil {
			return nil, nil, err
		}
	}

	region, regionExplanation, err := determineRegion(ctx, appConfig, org.PaidPlan)
	if err != nil {
		if err := tryRecoverErr(err); err != nil {
			return nil, nil, err
		}
	}

	if copiedConfig {
		// Check imported fly.toml is a valid V2 config before creating the app
		if err := appConfig.SetMachinesPlatform(); err != nil {
			return nil, nil, fmt.Errorf("can not use configuration for Fly Launch, check fly.toml: %w", err)
		}
		if flag.GetBool(ctx, "manifest") {
			fmt.Fprintln(iostreams.FromContext(ctx).ErrOut,
				"Warning: --manifest does not serialize an entire app configuration.\n"+
					"Creating a manifest from an existing fly.toml may be a lossy process!",
			)
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

	appName, appNameExplanation, err := determineAppName(ctx, appConfig, configPath)
	if err != nil {
		if err := tryRecoverErr(err); err != nil {
			return nil, nil, err
		}
	}

	guest, guestExplanation, err := determineGuest(ctx, appConfig, srcInfo)
	if err != nil {
		if err := tryRecoverErr(err); err != nil {
			return nil, nil, err
		}
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
		FlyctlVersion:    buildinfo.Info().Version,
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

	if len(recoverableInUiErrors) != 0 {

		var allErrors string
		for _, err := range recoverableInUiErrors {
			allErrors += fmt.Sprintf(" * %s\n", strings.ReplaceAll(err.String(), "\n", "\n   "))
		}
		err = recoverableInUiError{errors.New(allErrors)}
	}

	return &LaunchManifest{
			Plan:       lp,
			PlanSource: planSource,
		}, &planBuildCache{
			appConfig: appConfig,
			srcInfo:   srcInfo,
		}, err

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

	if taken, _ := appNameTaken(ctx, m.Plan.AppName); taken {
		return nil, appNameTakenErr(m.Plan.AppName)
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

// App names must consist of only lowercase letters, numbers, and dashes.
// Non-ascii characters are removed.
// Special characters are replaced with dashes, and sequences of dashes are collapsed into one.
func sanitizeAppName(dirName string) string {
	sanitized := make([]rune, 0, len(dirName))
	lastIsUnderscore := false

	for _, c := range dirName {
		if c <= unicode.MaxASCII {
			continue
		}
		if !unicode.IsLetter(c) && !unicode.IsNumber(c) {
			if !lastIsUnderscore {
				sanitized = append(sanitized, '-')
				lastIsUnderscore = true
			}
		} else {
			sanitized = append(sanitized, unicode.ToLower(c))
			lastIsUnderscore = false
		}
	}
	return strings.Trim(string(sanitized), "-")
}

func validateAppName(appName string) error {
	failRegex := regexp.MustCompile(`[^a-z0-9\-]`)
	if failRegex.MatchString(appName) {
		return errors.New("app name must consist of only lowercase letters, numbers, and dashes")
	}
	return nil
}

// determineAppName determines the app name from the config file or directory name
func determineAppName(ctx context.Context, appConfig *appconfig.Config, configPath string) (string, string, error) {

	delimiter := "-"
	findUniqueAppName := func(prefix string) (string, bool) {
		if prefix != "" {
			prefix += delimiter
		}
		for i := 1; i < 10; i++ {
			outName := prefix + haikunator.Haikunator().Delimiter(delimiter).String()
			if taken, _ := appNameTaken(ctx, outName); !taken {
				return outName, true
			}
		}
		return "", false
	}

	// This logic is a little overcomplicated. Essentially, just waterfall down options until one returns a valid name.
	//
	// Get initial name:
	//  1. If we've specified --name, use that name.
	//  2. If we've specified --generate-name, generate a unique name (meaning, jump over step 3)
	//  3. If we've provided an existing config file, use the app name from that.
	//  4. Use the directory name.
	//  5. If none of those sanitize into valid app names, generate one with Haikunator.
	// Ensure valid name:
	//  If the name is already taken, try to generate a unique suffix using Haikunator.
	//  If this fails, return a recoverable error.

	appName := flag.GetString(ctx, "name")
	if !flag.GetBool(ctx, "generate-name") && appName == "" {
		appName = appConfig.AppName
	}
	if appName == "" {
		appName = sanitizeAppName(filepath.Base(filepath.Dir(configPath)))
	}
	if appName == "" {

		var found bool
		appName, found = findUniqueAppName("")

		if !found {
			return "", recoverableSpecifyInUi, recoverableInUiError{flyerr.GenericErr{
				Err:     "unable to determine app name",
				Suggest: "You can specify the app name with the --name flag",
			}}
		}
	}
	if err := validateAppName(appName); err != nil {
		return "", recoverableSpecifyInUi, recoverableInUiError{err}
	}
	// If the app name is already taken, try to generate a unique suffix.
	if taken, _ := appNameTaken(ctx, appName); taken {

		var found bool
		appName, found = findUniqueAppName(appName)
		if !found {
			return "", recoverableSpecifyInUi, recoverableInUiError{appNameTakenErr(appName)}
		}
	}
	return appName, "derived from your directory name", nil
}

func appNameTaken(ctx context.Context, name string) (bool, error) {
	client := client.FromContext(ctx).API()
	// TODO: I believe this will only check apps that are visible to you.
	//       We should probably expose a global uniqueness check.
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
		return &personal, recoverableSpecifyInUi, recoverableInUiError{fmt.Errorf("organization '%s' not found", orgSlug)}
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

	// Get the closest region
	// TODO(allison): does this return paid regions for free orgs?
	closestRegion, closestRegionErr := client.API().GetNearestRegion(ctx)

	if regionCode != "" {
		region, err := getRegionByCode(ctx, regionCode)
		if err != nil {
			// Check and see if this is recoverable
			if closestRegionErr == nil {
				return closestRegion, recoverableSpecifyInUi, recoverableInUiError{err}
			}
		}
		return region, explanation, err
	}
	return closestRegion, "this is the fastest region for you", closestRegionErr
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
	def := helpers.Clone(api.MachinePresets["shared-cpu-1x"])
	def.MemoryMB = 1024
	reason := "most apps need about 1GB of RAM"

	guest, err := flag.GetMachineGuest(ctx, helpers.Clone(def))
	if err != nil {
		return def, recoverableSpecifyInUi, recoverableInUiError{err}
	}

	if def.CPUs != guest.CPUs || def.CPUKind != guest.CPUKind || def.MemoryMB != guest.MemoryMB {
		reason = "specified on the command line"
	}
	return guest, reason, nil
}
