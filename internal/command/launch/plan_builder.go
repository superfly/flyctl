package launch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"unicode"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/haikunator"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/scanner"
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
	appConfig  *appconfig.Config
	sourceInfo *scanner.SourceInfo
	// true means we've checked the app name, but not necessarily that it's okay. only that an error, if present, has been flagged already.
	// used to skip double-validating in stateFromManifest
	appNameValidated bool
	warnedNoCcHa     bool // true => We have already warned that deploying ha is impossible for an org with no payment method
}

func appNameTakenErr(appName string) error {
	return flyerr.GenericErr{
		Err:      fmt.Sprintf("app name %s is already taken", appName),
		Descript: "each Fly.io app must have a unique name",
		Suggest:  "Please specify a different app name with --name",
	}
}

type recoverableErrorBuilder struct {
	canEnterUi bool
	errors     []recoverableInUiError
}

func (r *recoverableErrorBuilder) tryRecover(e error) error {
	var asRecoverableErr recoverableInUiError
	if errors.As(e, &asRecoverableErr) && r.canEnterUi {
		r.errors = append(r.errors, asRecoverableErr)
		return nil
	}
	return e
}

func (r *recoverableErrorBuilder) build() string {
	if len(r.errors) == 0 {
		return ""
	}

	var allErrors string
	for _, err := range r.errors {
		allErrors += fmt.Sprintf(" * %s\n", strings.ReplaceAll(err.String(), "\n", "\n   "))
	}
	return allErrors
}

func buildManifest(ctx context.Context, parentConfig *appconfig.Config, recoverableErrors *recoverableErrorBuilder) (*LaunchManifest, *planBuildCache, error) {
	io := iostreams.FromContext(ctx)

	appConfig, copiedConfig, err := determineBaseAppConfig(ctx)
	if err != nil {
		return nil, nil, err
	}

	// TODO(allison): possibly add some automatic suffixing to app names if they already exist

	org, orgExplanation, err := determineOrg(ctx, parentConfig)
	if err != nil {
		if err := recoverableErrors.tryRecover(err); err != nil {
			return nil, nil, err
		}
	}

	region, regionExplanation, err := determineRegion(ctx, appConfig, org.PaidPlan)
	if err != nil {
		if err := recoverableErrors.tryRecover(err); err != nil {
			return nil, nil, err
		}
	}

	httpServicePort := 8080
	if copiedConfig {
		// Check imported fly.toml is a valid V2 config before creating the app
		if err := appConfig.SetMachinesPlatform(); err != nil {
			return nil, nil, fmt.Errorf("can not use configuration for Fly Launch, check fly.toml: %w", err)
		}
		if flag.GetBool(ctx, "manifest") {
			fmt.Fprintln(io.ErrOut,
				"Warning: --manifest does not serialize an entire app configuration.\n"+
					"Creating a manifest from an existing fly.toml may be a lossy process!",
			)
		}
		if service := appConfig.HTTPService; service != nil {
			httpServicePort = service.InternalPort
		} else {
			httpServicePort = 0
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

	appName, appNameExplanation, err := determineAppName(ctx, parentConfig, appConfig, configPath)
	if err != nil {
		if err := recoverableErrors.tryRecover(err); err != nil {
			return nil, nil, err
		}
	}

	compute, computeExplanation, err := determineCompute(ctx, appConfig, srcInfo)
	if err != nil {
		if err := recoverableErrors.tryRecover(err); err != nil {
			return nil, nil, err
		}
	}

	// HACK: This is a temporary solution to work around the fact that the UI doesn't
	//       understand the "compute" field. We want to move towards supporting the
	//       full compute definition at some point.
	appConfig.Compute = compute
	fakeDefaultMachine, err := appConfig.ToMachineConfig(appConfig.DefaultProcessName(), nil)
	if err != nil {
		return nil, nil, err
	}
	guest := fakeDefaultMachine.Guest

	// TODO: Determine databases requested by the sourceInfo, and add them to the plan.

	lp := &plan.LaunchPlan{
		AppName:          appName,
		OrgSlug:          org.Slug,
		RegionCode:       region.Code,
		HighAvailability: flag.GetBool(ctx, "ha"),
		Compute:          compute,
		CPUKind:          guest.CPUKind,
		CPUs:             guest.CPUs,
		MemoryMB:         guest.MemoryMB,
		VmSize:           guest.ToSize(),
		HttpServicePort:  httpServicePort,
		Postgres:         plan.PostgresPlan{},
		Redis:            plan.RedisPlan{},
		GitHubActions:    plan.GitHubActionsPlan{},
		FlyctlVersion:    buildinfo.Info().Version,
	}

	planSource := &launchPlanSource{
		appNameSource:  appNameExplanation,
		regionSource:   regionExplanation,
		orgSource:      orgExplanation,
		computeSource:  computeExplanation,
		postgresSource: "not requested",
		redisSource:    "not requested",
		tigrisSource:   "not requested",
		sentrySource:   "not requested",
	}

	buildCache := &planBuildCache{
		appConfig:        appConfig,
		sourceInfo:       srcInfo,
		appNameValidated: true, // validated in determineAppName
		warnedNoCcHa:     false,
	}

	if planValidateHighAvailability(ctx, lp, org, true) {
		buildCache.warnedNoCcHa = true
	}

	if srcInfo != nil {
		lp.ScannerFamily = srcInfo.Family
		const scannerSource = "determined from app source"
		if !flag.GetBool(ctx, "no-db") {
			switch srcInfo.DatabaseDesired {
			case scanner.DatabaseKindPostgres:
				lp.Postgres = plan.DefaultPostgres(lp)
				planSource.postgresSource = scannerSource
			case scanner.DatabaseKindMySQL:
				// TODO
			case scanner.DatabaseKindSqlite:
				// TODO
			}
		}
		if !flag.GetBool(ctx, "no-redis") && srcInfo.RedisDesired {
			lp.Redis = plan.DefaultRedis(lp)
			planSource.redisSource = scannerSource
		}
		if !flag.GetBool(ctx, "no-object-storage") && srcInfo.ObjectStorageDesired {
			lp.ObjectStorage = plan.DefaultObjectStorage(lp)
			planSource.tigrisSource = scannerSource
		}
		if srcInfo.Port != 0 {
			lp.HttpServicePort = srcInfo.Port
			lp.HttpServicePortSetByScanner = true
		}
		lp.Runtime = srcInfo.Runtime
	}

	return &LaunchManifest{
		Plan:       lp,
		PlanSource: planSource,
	}, buildCache, nil
}

// Check to see if they own the named app, and if so, prompt them to try deploying instead.
// Returns a whether or not the app is known to be taken.
// false doesn't necessarily mean available, just not visible to the user, while true means the app name is definitively taken.
// Returns an error only if the prompt library encounters an error. (this should never occur)
func nudgeTowardsDeploy(ctx context.Context, appName string) (bool, error) {

	client := flyutil.ClientFromContext(ctx)
	io := iostreams.FromContext(ctx)

	if flag.GetYes(ctx) {
		return false, nil
	}

	if _, err := client.GetApp(ctx, appName); err != nil {
		// The user can't see the app. Let them proceed.
		return false, nil
	}

	// The user can see the app. Prompt them to deploy.
	fmt.Fprintf(io.Out, "App '%s' already exists. You can deploy to it with `fly deploy`.\n", appName)

	switch confirmed, err := prompt.Confirm(ctx, "Continue launching a new app? "); {
	case err == nil:
		if !confirmed {
			// We've redirected the user to use 'fly deploy'
			// Exit directly with code 0 so this isn't flagged as a failed launch
			os.Exit(0)
		}
	case prompt.IsNonInteractive(err):
		// Should be impossible - we're only called if recoverableErrors.canEnterUi is true
		return true, nil
	default:
		return true, err
	}
	return true, nil
}

func stateFromManifest(ctx context.Context, m LaunchManifest, optionalCache *planBuildCache, recoverableErrors *recoverableErrorBuilder) (*launchState, error) {
	var (
		io     = iostreams.FromContext(ctx)
		client = flyutil.ClientFromContext(ctx)
	)

	org, err := client.GetOrganizationRemoteBuilderBySlug(ctx, m.Plan.OrgSlug)
	if err != nil {
		return nil, err
	}

	// If we potentially are deploying, launch a remote builder to prepare for deployment.
	if !flag.GetBool(ctx, "no-deploy") {
		// TODO: determine if eager remote builder is still required here
		go imgsrc.EagerlyEnsureRemoteBuilder(ctx, client, org, flag.GetRecreateBuilder(ctx))
	}

	var (
		appConfig        *appconfig.Config
		copiedConfig     bool
		warnedNoCcHa     bool
		appNameValidated bool
	)
	if optionalCache != nil {
		appConfig = optionalCache.appConfig
		warnedNoCcHa = optionalCache.warnedNoCcHa
		appNameValidated = optionalCache.appNameValidated
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

	// We don't check the app name being taken unless we can go to the UI, because
	// it'll fail when creating the app *anyway*, so unless you can use the UI it'll be the same result.
	if recoverableErrors.canEnterUi && !appNameValidated {
		taken, err := appNameTaken(ctx, m.Plan.AppName)
		if err != nil {
			return nil, flyerr.GenericErr{
				Err:     "unable to determine app name availability",
				Suggest: "Please try again in a minute",
			}
		}
		if taken {
			err := recoverableErrors.tryRecover(recoverableInUiError{appNameTakenErr(m.Plan.AppName)})
			if err != nil {
				return nil, err
			}
			m.PlanSource.appNameSource = recoverableSpecifyInUi
		}
	}

	workingDir := flag.GetString(ctx, "path")
	if absDir, err := filepath.Abs(workingDir); err == nil {
		workingDir = absDir
	}
	configPath := filepath.Join(workingDir, appconfig.DefaultConfigFileName)

	planStep := plan.GetPlanStep(ctx)
	if planStep == "" || planStep == "create" {
		fmt.Fprintln(io.Out, "Creating app in", workingDir)
	}

	var srcInfo *scanner.SourceInfo

	if optionalCache != nil {
		srcInfo = optionalCache.sourceInfo
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
		env: envVars,
		planBuildCache: planBuildCache{
			appConfig:    appConfig,
			sourceInfo:   srcInfo,
			warnedNoCcHa: warnedNoCcHa,
		},
		cache: map[string]interface{}{},
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

		// if --attach is specified, we should return the config as the base config
		attach := flag.GetBool(ctx, "attach")
		copyConfig := flag.GetBool(ctx, "copy-config") || attach

		if !flag.IsSpecified(ctx, "copy-config") && !attach && !flag.GetYes(ctx) {
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
		if c >= unicode.MaxASCII {
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
func determineAppName(ctx context.Context, parentConfig *appconfig.Config, appConfig *appconfig.Config, configPath string) (string, string, error) {
	delimiter := "-"
	findUniqueAppName := func(prefix string) (string, bool) {
		// Remove any existing haikus so we don't keep adding to the end.
		b := haikunator.Haikunator().Delimiter(delimiter)
		prefix = b.TrimSuffix(prefix)

		if prefix != "" {
			prefix += delimiter
		}
		for i := 1; i < 5; i++ {
			outName := prefix + b.String()
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
	//  2. If we've specified --generate-name, generate a unique name (meaning: jump to step 5)
	//  3. If we've provided an existing config file, use the app name from that.
	//  4. Use the directory name.
	//  5. If none of those sanitize into valid app names, generate one with Haikunator.
	// Ensure valid name:
	//  If the name is already taken, try to generate a unique suffix using Haikunator.
	//  If this fails, return a recoverable error.

	appName := flag.GetString(ctx, "name")
	cause := "specified on the command line"

	if !flag.GetBool(ctx, "generate-name") {
		// --generate-name wasn't specified, so we try to get a name from the config file or directory name.
		if appName == "" {
			appName = appConfig.AppName
			cause = "from your fly.toml"
		}
		if appName == "" {
			appName = sanitizeAppName(filepath.Base(filepath.Dir(configPath)))
			cause = "derived from your directory name"
		}

		if parentConfig != nil && parentConfig.AppName != "" {
			appName = parentConfig.AppName + "-" + appName
			if cause == "from your fly.toml" {
				cause = "from parent name and fly.toml"
			} else if cause == "derived from your directory name" {
				if flag.GetString(ctx, "into") != "" {
					cause = "from parent name and --into"
				} else if flag.GetString(ctx, "from") != "" {
					cause = "from parent name and --from"
				}
			}
		}
	}

	taken := appName == ""

	planStep := plan.GetPlanStep(ctx)
	if planStep != "" && planStep != "propose" && planStep != "create" {
		// We're not proposing a plan or creating an app, so we don't need to validate the app name.
		taken = false
	} else {
		if !taken && !flag.GetBool(ctx, "no-create") {
			var err error
			// If the user can see an app with the same name as what they're about to launch,
			// they *probably* want to deploy to that app instead.
			taken, err = nudgeTowardsDeploy(ctx, appName)
			if err != nil {
				return "", recoverableSpecifyInUi, recoverableInUiError{fmt.Errorf("failed to validate app name: %w", err)}
			}
		}

		if !taken {
			taken, _ = appNameTaken(ctx, appName)
		}
	}

	if taken {
		var found bool
		appName, found = findUniqueAppName(appName)
		cause = "generated"

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
	return appName, cause, nil
}

func appNameTaken(ctx context.Context, name string) (bool, error) {
	client := flyutil.ClientFromContext(ctx)

	available, err := client.AppNameAvailable(ctx, name)
	if err != nil {
		return false, err
	}
	return !available, nil
}

// determineOrg returns the org specified on the command line, or the personal org if left unspecified
func determineOrg(ctx context.Context, config *appconfig.Config) (*fly.Organization, string, error) {
	client := flyutil.ClientFromContext(ctx)

	if flag.GetBool(ctx, "attach") && config != nil && config.AppName != "" {
		org, err := client.GetOrganizationByApp(ctx, config.AppName)
		if err == nil {
			return org, fmt.Sprintf("from %s app", config.AppName), nil
		}
	}

	orgs, err := client.GetOrganizations(ctx)
	if err != nil {
		return nil, "", err
	}

	bySlug := make(map[string]fly.Organization, len(orgs))
	for _, o := range orgs {
		bySlug[o.Slug] = o
	}
	byName := make(map[string]fly.Organization, len(orgs))
	for _, o := range orgs {
		byName[o.Name] = o
	}

	personal, foundPersonal := bySlug["personal"]

	orgRequested := flag.GetOrg(ctx)
	if orgRequested == "" {
		if !foundPersonal {
			if len(orgs) == 0 {
				return nil, "", errors.New("no organizations found. Please create one from your fly dashboard first.")
			} else {
				o := orgs[0]
				return &o, fmt.Sprintf("defaulting to '%s'", o.Slug), nil
			}
		}

		return &personal, "fly launch defaults to the personal org", nil
	}

	org, foundSlug := bySlug[orgRequested]
	if !foundSlug {
		if org, foundName := byName[orgRequested]; foundName {
			return &org, "specified on the command line", nil
		}

		if !foundPersonal {
			return nil, "", errors.New("no personal organization found")
		}

		return &personal, recoverableSpecifyInUi, recoverableInUiError{fmt.Errorf("organization '%s' not found", orgRequested)}
	}

	return &org, "specified on the command line", nil
}

// determineRegion returns the region to use for a new app. In order, it tries:
//  1. the primary_region field of the config, if one exists
//  2. the region specified on the command line, if specified
//  3. the nearest region to the user
func determineRegion(ctx context.Context, config *appconfig.Config, paidPlan bool) (*fly.Region, string, error) {
	client := flyutil.ClientFromContext(ctx)
	regionCode := flag.GetRegion(ctx)
	explanation := "specified on the command line"

	if regionCode == "" {
		regionCode = config.PrimaryRegion
		explanation = "from your fly.toml"
	}

	// Get the closest region
	// TODO(allison): does this return paid regions for free orgs?
	closestRegion, closestRegionErr := client.GetNearestRegion(ctx)

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
func getRegionByCode(ctx context.Context, regionCode string) (*fly.Region, error) {
	apiClient := flyutil.ClientFromContext(ctx)

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

// Applies the fields of the guest to the provided compute.
// Ignores the guest's kernel arguments, host dedication, and GPU config,
// leaving whatever the given compute originally had.
//
// This is because this function is meant for backwards compatibility with
// the Web UI's guest definition, which doesn't have these fields.
func applyGuestToCompute(c *appconfig.Compute, g *fly.MachineGuest) {
	for k, v := range fly.MachinePresets {
		if reflect.DeepEqual(*v, *g) {
			c.MachineGuest = nil
			c.Memory = ""
			c.Size = k
			return
		}
	}

	originalGuest := c.MachineGuest
	clonedGuest := helpers.Clone(g)
	c.MachineGuest = clonedGuest

	// Canonicalize to human-readable memory strings when possible
	var memStr string
	if g.MemoryMB%1024 == 0 {
		memStr = fmt.Sprintf("%dgb", g.MemoryMB/1024)
	} else {
		memStr = fmt.Sprintf("%dmb", g.MemoryMB)
	}
	c.Memory = memStr
	c.MemoryMB = 0

	// Restore original values for fields the Web UI does not return
	if originalGuest != nil {
		c.MachineGuest.KernelArgs = originalGuest.KernelArgs
		c.MachineGuest.GPUs = originalGuest.GPUs
		c.MachineGuest.HostDedicationID = originalGuest.HostDedicationID
	}
}

func guestToCompute(g *fly.MachineGuest) *appconfig.Compute {
	var c appconfig.Compute
	applyGuestToCompute(&c, g)
	return &c
}

// determineCompute returns the guest type to use for a new app.
// Currently, it defaults to shared-cpu-1x
func determineCompute(ctx context.Context, config *appconfig.Config, srcInfo *scanner.SourceInfo) ([]*appconfig.Compute, string, error) {
	if len(config.Compute) > 0 {
		return config.Compute, "from your fly.toml", nil
	}

	def := helpers.Clone(fly.MachinePresets["shared-cpu-1x"])
	def.MemoryMB = 1024
	reason := "most apps need about 1GB of RAM"

	guest, err := flag.GetMachineGuest(ctx, helpers.Clone(def))
	if err != nil {
		return []*appconfig.Compute{guestToCompute(def)}, recoverableSpecifyInUi, recoverableInUiError{err}
	}

	if def.CPUs != guest.CPUs || def.CPUKind != guest.CPUKind || def.MemoryMB != guest.MemoryMB {
		reason = "specified on the command line"
	}
	return []*appconfig.Compute{guestToCompute(guest)}, reason, nil
}

func planValidateHighAvailability(ctx context.Context, p *plan.LaunchPlan, org *fly.Organization, print bool) bool {
	if !org.Billable && p.HighAvailability {
		if print {
			fmt.Fprintln(iostreams.FromContext(ctx).ErrOut, "Warning: This organization has no payment method, turning off high availability")
		}
		return false
	}
	return true
}
