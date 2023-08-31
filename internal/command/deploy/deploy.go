package deploy

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/command/machine"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/sentry"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/watch"
)

var CommonFlags = flag.Set{
	flag.Region(),
	flag.Image(),
	flag.Now(),
	flag.RemoteOnly(false),
	flag.LocalOnly(),
	flag.Push(),
	flag.Detach(),
	flag.Strategy(),
	flag.Dockerfile(),
	flag.Ignorefile(),
	flag.ImageLabel(),
	flag.BuildArg(),
	flag.BuildSecret(),
	flag.BuildTarget(),
	flag.NoCache(),
	flag.Nixpacks(),
	flag.BuildOnly(),
	flag.Bool{
		Name:        "provision-extensions",
		Description: "Provision any extensions assigned as a default to first deployments",
	},
	flag.Bool{
		Name:        "no-extensions",
		Description: "Do not provision Sentry nor other auto-provisioned extensions",
	},
	flag.StringArray{
		Name:        "env",
		Shorthand:   "e",
		Description: "Set of environment variables in the form of NAME=VALUE pairs. Can be specified multiple times.",
	},
	flag.Yes(),
	flag.Int{
		Name:        "wait-timeout",
		Description: "Seconds to wait for individual machines to transition states and become healthy.",
		Default:     int(DefaultWaitTimeout.Seconds()),
	},
	flag.String{
		Name:        "release-command-timeout",
		Description: "Seconds to wait for a release command finish running, or 'none' to disable.",
		Default:     strconv.Itoa(int(DefaultReleaseCommandTimeout.Seconds())),
	},
	flag.Int{
		Name:        "lease-timeout",
		Description: "Seconds to lease individual machines while running deployment. All machines are leased at the beginning and released at the end. The lease is refreshed periodically for this same time, which is why it is short. flyctl releases leases in most cases.",
		Default:     int(DefaultLeaseTtl.Seconds()),
	},
	flag.Bool{
		Name:        "force-nomad",
		Description: "(Deprecated) Use the Apps v1 platform built with Nomad",
		Default:     false,
		Hidden:      true,
	},
	flag.Bool{
		Name:        "force-machines",
		Description: "Use the Apps v2 platform built with Machines",
		Default:     false,
		Hidden:      true,
	},
	flag.Bool{
		Name:        "ha",
		Description: "Create spare machines that increases app availability",
		Default:     true,
	},
	flag.Bool{
		Name:        "smoke-checks",
		Description: "Perform smoke checks during deployment",
		Default:     true,
	},
	flag.Float64{
		Name:        "max-unavailable",
		Description: "Max number of unavailable machines during rolling updates. A number between 0 and 1 means percent of total machines",
		Default:     0.33,
	},
	flag.Bool{
		Name:        "no-public-ips",
		Description: "Do not allocate any new public IP addresses",
	},
	flag.StringArray{
		Name:        "file-local",
		Description: "Set of files in the form of /path/inside/machine=<local/path> pairs. Can be specified multiple times.",
	},
	flag.StringArray{
		Name:        "file-literal",
		Description: "Set of literals in the form of /path/inside/machine=VALUE pairs where VALUE is the content. Can be specified multiple times.",
	},
	flag.StringArray{
		Name:        "file-secret",
		Description: "Set of secrets in the form of /path/inside/machine=SECRET pairs where SECRET is the name of the secret. Can be specified multiple times.",
	},
	flag.StringSlice{
		Name:        "exclude-regions",
		Description: "Deploy to all machines except machines in these regions. Multiple regions can be specified with comma separated values or by providing the flag multiple times. --exclude-regions iad,sea --exclude-regions syd will exclude all three iad, sea, and syd regions. Applied after --only-regions. V2 machines platform only.",
	},
	flag.StringSlice{
		Name:        "only-regions",
		Description: "Deploy to machines only in these regions. Multiple regions can be specified with comma separated values or by providing the flag multiple times. --only-regions iad,sea --only-regions syd will deploy to all three iad, sea, and syd regions. Applied before --exclude-regions. V2 machines platform only.",
	},
	flag.VMSizeFlags,
}

func New() (cmd *cobra.Command) {
	const (
		long = `Deploy Fly applications from source or an image using a local or remote builder.

		To disable colorized output and show full Docker build output, set the environment variable NO_COLOR=1.
	`
		short = "Deploy Fly applications"
	)

	cmd = command.New("deploy [WORKING_DIRECTORY]", short, long, run,
		command.RequireSession,
		command.ChangeWorkingDirectoryToFirstArgIfPresent,
		command.RequireAppName,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		CommonFlags,
		flag.App(),
		flag.AppConfig(),
		// Not in CommonFlags because it's not relevant to a first deploy
		flag.Bool{
			Name:        "update-only",
			Description: "Do not create Machines for new process groups",
			Default:     false,
		},
	)

	return
}

func run(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)
	flapsClient, err := flaps.NewFromAppName(ctx, appName)
	if err != nil {
		return fmt.Errorf("could not create flaps client: %w", err)
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	appConfig, err := determineAppConfig(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "Could not find App") {
			return fmt.Errorf("the app name %s could not be found, did you create the app or misspell it in the fly.toml file or via -a?", appName)
		}
		return err
	}

	return DeployWithConfig(ctx, appConfig, flag.GetYes(ctx), nil)
}

func DeployWithConfig(ctx context.Context, appConfig *appconfig.Config, forceYes bool, optionalGuest *api.MachineGuest) (err error) {
	io := iostreams.FromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	apiClient := client.FromContext(ctx).API()
	appCompact, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	for _, potentialSecretSubstr := range commonSecretSubstrings {
		for env := range appConfig.Env {
			if strings.Contains(env, potentialSecretSubstr) {
				warning := fmt.Sprintf("%s %s may be a potentially sensitive environment variable. Consider setting it as a secret, and removing it from the [env] section: https://fly.io/docs/reference/secrets/\n", aurora.Yellow("WARN"), env)
				fmt.Fprintln(io.ErrOut, warning)
			}
		}
	}

	// Fetch an image ref or build from source to get the final image reference to deploy
	img, err := determineImage(ctx, appConfig)
	if err != nil {
		return fmt.Errorf("failed to fetch an image or build from source: %w", err)
	}

	if flag.GetBuildOnly(ctx) {
		return nil
	}

	fmt.Fprintf(io.Out, "\nWatch your deployment at https://fly.io/apps/%s/monitoring\n\n", appName)
	if useMachines(ctx, appCompact) {
		if err := appConfig.EnsureV2Config(); err != nil {
			return fmt.Errorf("Can't deploy an invalid v2 app config: %s", err)
		}
		if err := deployToMachines(ctx, appConfig, appCompact, img, optionalGuest); err != nil {
			return err
		}
	} else {
		if flag.GetBool(ctx, "no-public-ips") {
			return fmt.Errorf("the --no-public-ips flag can only be used for v2 apps")
		}
		if flag.IsSpecified(ctx, "vm-cpus") {
			return fmt.Errorf("the --vm-cpus flag can only be used for v2 apps")
		}
		if flag.IsSpecified(ctx, "vm-memory") {
			return fmt.Errorf("the --vm-memory flag can only be used for v2 apps")
		}

		err = deployToNomad(ctx, appConfig, appCompact, img)
		if err != nil {
			return err
		}
	}

	if appURL := appConfig.URL(); appURL != nil {
		fmt.Fprintf(io.Out, "\nVisit your newly deployed app at %s\n", appURL)
	}

	return err
}

func determineRelCmdTimeout(timeout string) (time.Duration, error) {
	if timeout == "none" {
		return 0, nil
	}
	asInt, err := strconv.Atoi(timeout)
	if err != nil {
		return 0, fmt.Errorf("invalid release command timeout '%v': valid options are a number of seconds, or 'none'", timeout)
	}
	return time.Duration(asInt) * time.Second, nil
}

// in a rare twist, the guest param takes precedence over CLI flags!
func deployToMachines(
	ctx context.Context,
	appConfig *appconfig.Config,
	appCompact *api.AppCompact,
	img *imgsrc.DeploymentImage,
	guest *api.MachineGuest,
) (err error) {
	// It's important to push appConfig into context because MachineDeployment will fetch it from there
	ctx = appconfig.WithConfig(ctx, appConfig)

	metrics.Started(ctx, "deploy_machines")
	defer func() {
		metrics.Status(ctx, "deploy_machines", err == nil)
	}()

	releaseCmdTimeout, err := determineRelCmdTimeout(flag.GetString(ctx, "release-command-timeout"))
	if err != nil {
		return err
	}

	files, err := machine.FilesFromCommand(ctx)
	if err != nil {
		return err
	}

	if guest == nil {
		guest = flag.GetMachineGuest(ctx)
	}

	excludeRegions := make(map[string]interface{})
	for _, r := range flag.GetStringSlice(ctx, "exclude-regions") {
		reg := strings.TrimSpace(r)
		if reg != "" {
			excludeRegions[reg] = struct{}{}
		}
	}
	onlyRegions := make(map[string]interface{})
	for _, r := range flag.GetStringSlice(ctx, "only-regions") {
		reg := strings.TrimSpace(r)
		if reg != "" {
			onlyRegions[reg] = struct{}{}
		}
	}

	md, err := NewMachineDeployment(ctx, MachineDeploymentArgs{
		AppCompact:            appCompact,
		DeploymentImage:       img.Tag,
		Strategy:              flag.GetString(ctx, "strategy"),
		EnvFromFlags:          flag.GetStringArray(ctx, "env"),
		PrimaryRegionFlag:     appConfig.PrimaryRegion,
		SkipSmokeChecks:       flag.GetDetach(ctx) || !flag.GetBool(ctx, "smoke-checks"),
		SkipHealthChecks:      flag.GetDetach(ctx),
		WaitTimeout:           time.Duration(flag.GetInt(ctx, "wait-timeout")) * time.Second,
		LeaseTimeout:          time.Duration(flag.GetInt(ctx, "lease-timeout")) * time.Second,
		MaxUnavailable:        flag.GetFloat64(ctx, "max-unavailable"),
		ReleaseCmdTimeout:     releaseCmdTimeout,
		Guest:                 guest,
		IncreasedAvailability: flag.GetBool(ctx, "ha"),
		AllocPublicIP:         !flag.GetBool(ctx, "no-public-ips"),
		UpdateOnly:            flag.GetBool(ctx, "update-only"),
		Files:                 files,
		ExcludeRegions:        excludeRegions,
		NoExtensions:          flag.GetBool(ctx, "no-extensions"),
		OnlyRegions:           onlyRegions,
	})
	if err != nil {
		sentry.CaptureExceptionWithAppInfo(err, "deploy", appCompact)
		return err
	}

	err = md.DeployMachinesApp(ctx)
	if err != nil {
		sentry.CaptureExceptionWithAppInfo(err, "deploy", appCompact)
	}
	return err
}

func deployToNomad(ctx context.Context, appConfig *appconfig.Config, appCompact *api.AppCompact, img *imgsrc.DeploymentImage) (err error) {
	apiClient := client.FromContext(ctx).API()

	metrics.Started(ctx, "deploy_nomad")
	defer func() {
		metrics.Status(ctx, "deploy_nomad", err == nil)
	}()

	// Assign an empty map if nil so later assignments won't fail
	if appConfig.PrimaryRegion != "" && appConfig.Env["PRIMARY_REGION"] == "" {
		appConfig.SetEnvVariable("PRIMARY_REGION", appConfig.PrimaryRegion)
	}

	release, releaseCommand, err := createRelease(ctx, appConfig, img)
	if err != nil {
		return err
	}

	// Give a warning about nomad deprecation every 5 releases
	if release.Version%5 == 0 {
		command.PromptToMigrate(ctx, appCompact)
	}

	if flag.GetDetach(ctx) {
		return nil
	}

	// TODO: This is a single message that doesn't belong to any block output, so we should have helpers to allow that
	tb := render.NewTextBlock(ctx)
	tb.Done("You can detach the terminal anytime without stopping the deployment")

	// Run the pre-deployment release command if it's set
	if releaseCommand != nil {
		// TODO: don't use text block here
		tb := render.NewTextBlock(ctx, fmt.Sprintf("Release command detected: %s\n", releaseCommand.Command))
		tb.Done("This release will not be available until the release command succeeds.")

		if err := watch.ReleaseCommand(ctx, appConfig.AppName, releaseCommand.ID); err != nil {
			return err
		}

		release, err = apiClient.GetAppReleaseNomad(ctx, appConfig.AppName, release.ID)
		if err != nil {
			return err
		}
	}

	if release.DeploymentStrategy == "IMMEDIATE" {
		logger := logger.FromContext(ctx)
		logger.Debug("immediate deployment strategy, nothing to monitor")

		return nil
	}

	return watch.Deployment(ctx, appConfig.AppName, release.EvaluationID)
}

func useMachines(ctx context.Context, appCompact *api.AppCompact) bool {
	if buildinfo.IsDev() && flag.GetBool(ctx, "force-nomad") && !appCompact.Deployed {
		return false
	}
	if appCompact.Deployed && appCompact.PlatformVersion == appconfig.NomadPlatform {
		return false
	}
	return true
}

// determineAppConfig fetches the app config from a local file, or in its absence, from the API
func determineAppConfig(ctx context.Context) (cfg *appconfig.Config, err error) {
	io := iostreams.FromContext(ctx)
	tb := render.NewTextBlock(ctx, "Verifying app config")
	appName := appconfig.NameFromContext(ctx)

	if cfg = appconfig.ConfigFromContext(ctx); cfg == nil {
		cfg, err = appconfig.FromRemoteApp(ctx, appName)
		if err != nil {
			return nil, err
		}
	}

	if env := flag.GetStringArray(ctx, "env"); len(env) > 0 {
		parsedEnv, err := cmdutil.ParseKVStringsToMap(env)
		if err != nil {
			return nil, fmt.Errorf("failed parsing environment: %w", err)
		}
		cfg.SetEnvVariables(parsedEnv)
	}

	// FIXME: this is a confusing flag; I thought it meant only update machines in the provided region, which resulted in a minor disaster :-)
	if v := flag.GetRegion(ctx); v != "" {
		cfg.PrimaryRegion = v
	}

	// Always prefer the app name passed via --app
	if appName != "" {
		cfg.AppName = appName
	}

	err, extraInfo := cfg.Validate(ctx)
	if extraInfo != "" {
		fmt.Fprintf(io.Out, extraInfo)
	}
	if err != nil {
		return nil, err
	}

	tb.Done("Verified app config")
	return cfg, nil
}

func createRelease(ctx context.Context, appConfig *appconfig.Config, img *imgsrc.DeploymentImage) (*api.Release, *api.ReleaseCommand, error) {
	tb := render.NewTextBlock(ctx, "Creating release")

	input := api.DeployImageInput{
		AppID: appConfig.AppName,
		Image: img.Tag,
	}

	// Set the deployment strategy
	if val := flag.GetString(ctx, "strategy"); val != "" {
		input.Strategy = api.StringPointer(strings.ReplaceAll(strings.ToUpper(val), "-", "_"))
	}

	input.Definition = api.DefinitionPtr(appConfig.SanitizedDefinition())

	// Start deployment of the determined image
	client := client.FromContext(ctx).API()

	release, releaseCommand, err := client.DeployImage(ctx, input)
	if err == nil {
		tb.Donef("release v%d created\n", release.Version)
	}

	return release, releaseCommand, err
}
