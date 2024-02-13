package deploy

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/superfly/fly-go/api"
	"github.com/superfly/fly-go/client"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/ctrlc"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/sentry"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
	"go.opentelemetry.io/otel/attribute"
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
	flag.BpDockerHost(),
	flag.BpVolume(),
	flag.Bool{
		Name:        "provision-extensions",
		Description: "Provision any extensions assigned as a default to first deployments",
	},
	flag.StringArray{
		Name:        "env",
		Shorthand:   "e",
		Description: "Set of environment variables in the form of NAME=VALUE pairs. Can be specified multiple times.",
	},
	flag.Yes(),
	flag.String{
		Name:        "wait-timeout",
		Description: "Time duration to wait for individual machines to transition states and become healthy.",
		Default:     DefaultWaitTimeout.String(),
	},
	flag.String{
		Name:        "release-command-timeout",
		Description: "Time duration to wait for a release command finish running, or 'none' to disable.",
		Default:     DefaultReleaseCommandTimeout.String(),
	},
	flag.String{
		Name: "lease-timeout",
		Description: "Time duration to lease individual machines while running deployment." +
			" All machines are leased at the beginning and released at the end." +
			"The lease is refreshed periodically for this same time, which is why it is short." +
			"flyctl releases leases in most cases.",
		Default: DefaultLeaseTtl.String(),
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
	flag.Bool{
		Name:        "dns-checks",
		Description: "Perform DNS checks during deployment",
		Default:     true,
	},
	flag.Float64{
		Name:        "max-unavailable",
		Description: "Max number of unavailable machines during rolling updates. A number between 0 and 1 means percent of total machines",
		Default:     DefaultMaxUnavailable,
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
	flag.StringArray{
		Name:        "label",
		Description: "Add custom metadata to an image via docker labels",
	},
	flag.Int{
		Name:        "immediate-max-concurrent",
		Description: "Maximum number of machines to update concurrently when using the immediate deployment strategy.",
		Default:     16,
	},
	flag.Int{
		Name:        "volume-initial-size",
		Description: "The initial size in GB for volumes created on first deploy",
	},
	flag.VMSizeFlags,
	flag.StringSlice{
		Name:        "process-groups",
		Description: "Deploy to machines only in these process groups",
	},
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
	hook := ctrlc.Hook(func() {
		metrics.FlushMetrics(ctx)
	})

	defer hook.Done()

	appName := appconfig.NameFromContext(ctx)
	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName:   appName,
		UserAgent: buildinfo.UserAgent(),
	})
	if err != nil {
		return fmt.Errorf("could not create flaps client: %w", err)
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	client := client.FromContext(ctx).API()

	ctx, span := tracing.CMDSpan(ctx, appName, "cmd.deploy")
	defer span.End()

	user, err := client.GetCurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("failed retrieving current user: %w", err)
	}

	span.SetAttributes(attribute.String("user.id", user.ID))

	appConfig, err := determineAppConfig(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "Could not find App") {
			return fmt.Errorf("the app name %s could not be found, did you create the app or misspell it in the fly.toml file or via -a?", appName)
		}
		return err
	}

	var gpuKinds, cpuKinds []string
	for _, compute := range appConfig.Compute {
		if compute != nil && compute.MachineGuest != nil {
			gpuKinds = append(gpuKinds, compute.MachineGuest.GPUKind)
			cpuKinds = append(cpuKinds, compute.MachineGuest.CPUKind)
		}
	}

	span.SetAttributes(attribute.StringSlice("gpu.kinds", gpuKinds))
	span.SetAttributes(attribute.StringSlice("cpu.kinds", cpuKinds))

	return DeployWithConfig(ctx, appConfig, flag.GetYes(ctx))
}

func DeployWithConfig(ctx context.Context, appConfig *appconfig.Config, forceYes bool) (err error) {
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
	if err := deployToMachines(ctx, appConfig, appCompact, img); err != nil {
		return err
	}

	if appURL := appConfig.URL(); appURL != nil {
		fmt.Fprintf(io.Out, "\nVisit your newly deployed app at %s\n", appURL)
	}

	return err
}

func parseDurationFlag(ctx context.Context, flagName string) (*time.Duration, error) {
	if !flag.IsSpecified(ctx, flagName) {
		return nil, nil
	}

	v := flag.GetString(ctx, flagName)
	if v == "none" {
		d := time.Duration(0)
		return &d, nil
	}

	duration, err := time.ParseDuration(v)
	if err == nil {
		return &duration, nil
	}

	if strings.Contains(err.Error(), "missing unit in duration") {
		asInt, err := strconv.Atoi(v)
		if err == nil {
			duration = time.Duration(asInt) * time.Second
			return &duration, nil
		}
	}

	return nil, fmt.Errorf("invalid duration value %v used for --%s flag: valid options are a number of seconds, number with time unit (i.e.: 5m, 180s) or 'none'", v, flagName)
}

// in a rare twist, the guest param takes precedence over CLI flags!
func deployToMachines(
	ctx context.Context,
	appConfig *appconfig.Config,
	appCompact *api.AppCompact,
	img *imgsrc.DeploymentImage,
) (err error) {
	// It's important to push appConfig into context because MachineDeployment will fetch it from there
	ctx = appconfig.WithConfig(ctx, appConfig)

	metrics.Started(ctx, "deploy_machines")
	defer func() {
		metrics.Status(ctx, "deploy_machines", err == nil)
	}()

	releaseCmdTimeout, err := parseDurationFlag(ctx, "release-command-timeout")
	if err != nil {
		return err
	}

	waitTimeout, err := parseDurationFlag(ctx, "wait-timeout")
	if err != nil {
		return err
	}

	leaseTimeout, err := parseDurationFlag(ctx, "lease-timeout")
	if err != nil {
		return err
	}

	files, err := command.FilesFromCommand(ctx)
	if err != nil {
		return err
	}

	guest, err := flag.GetMachineGuest(ctx, nil)
	if err != nil {
		return err
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

	// We default the flag to 0.33 so that --help can show the actual default value,
	// but internally we want to differentiate between the flag being specified and not.
	// We use 0.0 to denote unspecified, as that value is invalid for maxUnavailable.
	var maxUnavailable *float64 = nil
	if flag.IsSpecified(ctx, "max-unavailable") {
		maxUnavailable = api.Pointer(flag.GetFloat64(ctx, "max-unavailable"))
		// Validation to ensure that 0.0 is *purely* the "unspecified" value
		if *maxUnavailable <= 0 {
			return fmt.Errorf("the value for --max-unavailable must be > 0")
		}
	}

	processGroups := make(map[string]interface{})
	for _, r := range flag.GetStringSlice(ctx, "process-groups") {
		reg := strings.TrimSpace(r)
		if reg != "" {
			processGroups[reg] = struct{}{}
		}
	}

	md, err := NewMachineDeployment(ctx, MachineDeploymentArgs{
		AppCompact:             appCompact,
		DeploymentImage:        img.Tag,
		Strategy:               flag.GetString(ctx, "strategy"),
		EnvFromFlags:           flag.GetStringArray(ctx, "env"),
		PrimaryRegionFlag:      appConfig.PrimaryRegion,
		SkipSmokeChecks:        flag.GetDetach(ctx) || !flag.GetBool(ctx, "smoke-checks"),
		SkipHealthChecks:       flag.GetDetach(ctx),
		SkipDNSChecks:          flag.GetDetach(ctx) || !flag.GetBool(ctx, "dns-checks"),
		WaitTimeout:            waitTimeout,
		ReleaseCmdTimeout:      releaseCmdTimeout,
		LeaseTimeout:           leaseTimeout,
		MaxUnavailable:         maxUnavailable,
		Guest:                  guest,
		IncreasedAvailability:  flag.GetBool(ctx, "ha"),
		AllocPublicIP:          !flag.GetBool(ctx, "no-public-ips"),
		UpdateOnly:             flag.GetBool(ctx, "update-only"),
		Files:                  files,
		ExcludeRegions:         excludeRegions,
		OnlyRegions:            onlyRegions,
		ImmediateMaxConcurrent: flag.GetInt(ctx, "immediate-max-concurrent"),
		VolumeInitialSize:      flag.GetInt(ctx, "volume-initial-size"),
		ProcessGroups:          processGroups,
	})
	if err != nil {
		sentry.CaptureExceptionWithAppInfo(ctx, err, "deploy", appCompact)
		return err
	}

	err = md.DeployMachinesApp(ctx)
	if err != nil {
		sentry.CaptureExceptionWithAppInfo(ctx, err, "deploy", appCompact)
	}
	return err
}

// determineAppConfig fetches the app config from a local file, or in its absence, from the API
func determineAppConfig(ctx context.Context) (cfg *appconfig.Config, err error) {
	io := iostreams.FromContext(ctx)
	tb := render.NewTextBlock(ctx, "Verifying app config")
	appName := appconfig.NameFromContext(ctx)
	ctx, span := tracing.GetTracer().Start(ctx, "get_app_config")
	defer span.End()

	if cfg = appconfig.ConfigFromContext(ctx); cfg == nil {
		cfg, err = appconfig.FromRemoteApp(ctx, appName)
		if err != nil {
			tracing.RecordError(span, err, "get config from remote")
			return nil, err
		}
	}

	if env := flag.GetStringArray(ctx, "env"); len(env) > 0 {
		parsedEnv, err := cmdutil.ParseKVStringsToMap(env)
		if err != nil {
			tracing.RecordError(span, err, "parse env")
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
		tracing.RecordError(span, err, "validate config")
		return nil, err
	}

	if cfg.Deploy != nil && cfg.Deploy.Strategy != "rolling" && cfg.Deploy.MaxUnavailable != nil {
		if !config.FromContext(ctx).JSONOutput {
			fmt.Fprintf(io.Out, "Warning: max-unavailable set for non-rolling strategy '%s', ignoring\n", cfg.Deploy.Strategy)
		}
	}

	tb.Done("Verified app config")
	return cfg, nil
}
