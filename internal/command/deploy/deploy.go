package deploy

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
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
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/launchdarkly"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/sentry"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var defaultMaxConcurrent = 8

var CommonFlags = flag.Set{
	flag.Image(),
	flag.Now(),
	flag.RemoteOnly(false),
	flag.LocalOnly(),
	flag.Push(),
	flag.Wireguard(),
	flag.HttpsFailover(),
	flag.Detach(),
	flag.Strategy(),
	flag.Dockerfile(),
	flag.Ignorefile(),
	flag.ImageLabel(),
	flag.BuildArg(),
	flag.BuildSecret(),
	flag.BuildTarget(),
	flag.NoCache(),
	flag.Depot(),
	flag.DepotScope(),
	flag.Nixpacks(),
	flag.BuildOnly(),
	flag.BpDockerHost(),
	flag.BpVolume(),
	flag.RecreateBuilder(),
	flag.Yes(),
	flag.VMSizeFlags,
	flag.Env(),
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
	flag.Bool{
		Name:        "flycast",
		Description: "Allocate a private IPv6 addresses",
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
	flag.String{
		Name:        "primary-region",
		Description: "Override primary region in fly.toml configuration.",
	},
	flag.StringSlice{
		Name:        "regions",
		Aliases:     []string{"only-regions"},
		Description: "Deploy to machines only in these regions. Multiple regions can be specified with comma separated values or by providing the flag multiple times.",
	},
	flag.StringSlice{
		Name:        "exclude-regions",
		Description: "Deploy to all machines except machines in these regions. Multiple regions can be specified with comma separated values or by providing the flag multiple times.",
	},
	flag.StringSlice{
		Name:        "only-machines",
		Description: "Deploy to machines only with these IDs. Multiple IDs can be specified with comma separated values or by providing the flag multiple times.",
	},
	flag.StringSlice{
		Name:        "exclude-machines",
		Description: "Deploy to all machines except machines with these IDs. Multiple IDs can be specified with comma separated values or by providing the flag multiple times.",
	},
	flag.StringSlice{
		Name:        "process-groups",
		Description: "Deploy to machines only in these process groups",
	},
	flag.StringArray{
		Name:        "label",
		Description: "Add custom metadata to an image via docker labels",
	},
	flag.Int{
		Name:        "max-concurrent",
		Description: "Maximum number of machines to operate on concurrently.",
		Default:     defaultMaxConcurrent,
	},
	flag.Int{
		Name:        "immediate-max-concurrent",
		Description: "Maximum number of machines to update concurrently when using the immediate deployment strategy.",
		Default:     defaultMaxConcurrent,
		Hidden:      true,
	},
	flag.Int{
		Name:        "volume-initial-size",
		Description: "The initial size in GB for volumes created on first deploy",
	},
	flag.String{
		Name:        "signal",
		Shorthand:   "s",
		Description: "Signal to stop the machine with for bluegreen strategy (default: SIGINT)",
	},
	flag.String{
		Name:        "deploy-retries",
		Description: "Number of times to retry a deployment if it fails",
		Default:     "auto",
	},
}

type Command struct {
	*cobra.Command
}

func New() *Command {
	const (
		long = `Deploy Fly applications from source or an image using a local or remote builder.

		To disable colorized output and show full Docker build output, set the environment variable NO_COLOR=1.
	`
		short = "Deploy Fly applications"
	)

	cmd := &Command{}
	cmd.Command = command.New("deploy [WORKING_DIRECTORY]", short, long, cmd.run,
		command.RequireSession,
		command.ChangeWorkingDirectoryToFirstArgIfPresent,
		command.RequireAppName,
	)
	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd.Command,
		CommonFlags,
		flag.App(),
		flag.AppConfig(),
		// Not in CommonFlags because it's not relevant to a first deploy
		flag.Bool{
			Name:        "update-only",
			Description: "Do not create Machines for new process groups",
			Default:     false,
		},
		flag.Bool{
			Name:        "skip-release-command",
			Description: "Do not run the release command during deployment.",
			Default:     false,
		},
		flag.String{
			Name:        "export-manifest",
			Description: "Specify a file to export the deployment configuration to a deploy manifest file, or '-' to print to stdout.",
			Hidden:      true,
		},
		flag.String{
			Name:        "from-manifest",
			Description: "Path to a deploy manifest file to use for deployment.",
			Hidden:      true,
		},
	)

	return cmd
}

func (cmd *Command) run(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)
	appName := appconfig.NameFromContext(ctx)

	hook := ctrlc.Hook(func() {
		metrics.FlushMetrics(ctx)
	})

	defer hook.Done()

	tp, err := tracing.InitTraceProvider(ctx, appName)
	if err != nil {
		fmt.Fprintf(io.ErrOut, "failed to initialize tracing library: =%v", err)
		return err
	}

	defer tp.Shutdown(ctx)

	ctx, span := tracing.CMDSpan(ctx, "cmd.deploy")
	defer span.End()

	defer func() {
		if err != nil {
			tracing.RecordError(span, err, "error deploying")
		}
	}()

	// Instantiate FLAPS client if we haven't initialized one via a unit test.
	if flapsutil.ClientFromContext(ctx) == nil {
		flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
			AppName: appName,
		})
		if err != nil {
			return fmt.Errorf("could not create flaps client: %w", err)
		}
		ctx = flapsutil.NewContextWithClient(ctx, flapsClient)
	}

	client := flyutil.ClientFromContext(ctx)

	user, err := client.GetCurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("failed retrieving current user: %w", err)
	}

	span.SetAttributes(attribute.String("user.id", user.ID))

	var manifestPath = flag.GetString(ctx, "from-manifest")

	switch {
	case manifestPath == "-":
		manifest, err := manifestFromReader(io.In)
		if err != nil {
			return err
		}
		return deployFromManifest(ctx, manifest)
	case manifestPath != "":
		manifest, err := manifestFromFile(manifestPath)
		if err != nil {
			return err
		}
		return deployFromManifest(ctx, manifest)
	}

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

	err = DeployWithConfig(ctx, appConfig, 0, flag.GetYes(ctx))
	return err
}

func DeployWithConfig(ctx context.Context, appConfig *appconfig.Config, userID int, forceYes bool) (err error) {
	span := trace.SpanFromContext(ctx)

	io := iostreams.FromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	apiClient := flyutil.ClientFromContext(ctx)
	appCompact, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	// Start the feature flag client, if we haven't already
	if launchdarkly.ClientFromContext(ctx) == nil {
		ffClient, err := launchdarkly.NewClient(ctx, launchdarkly.UserInfo{
			OrganizationID: appCompact.Organization.InternalNumericID,
			UserID:         userID,
		})
		if err != nil {
			return fmt.Errorf("could not create feature flag client: %w", err)
		}
		ctx = launchdarkly.NewContextWithClient(ctx, ffClient)
	}

	for env := range appConfig.Env {
		if containsCommonSecretSubstring(env) {
			warning := fmt.Sprintf("%s %s may be a potentially sensitive environment variable. Consider setting it as a secret, and removing it from the [env] section: https://fly.io/docs/apps/secrets/\n", aurora.Yellow("WARN"), env)
			fmt.Fprintln(io.ErrOut, warning)
		}
	}

	httpFailover := flag.GetHTTPSFailover(ctx)
	usingWireguard := flag.GetWireguard(ctx)
	recreateBuilder := flag.GetRecreateBuilder(ctx)

	// Fetch an image ref or build from source to get the final image reference to deploy
	img, err := determineImage(ctx, appConfig, usingWireguard, recreateBuilder)
	if err != nil {
		noBuilder := strings.Contains(err.Error(), "Could not find App")
		recreateBuilder = recreateBuilder || noBuilder
		if noBuilder || (usingWireguard && httpFailover) {
			span.SetAttributes(attribute.String("builder.failover_error", err.Error()))
			span.AddEvent("using http failover")
			img, err = determineImage(ctx, appConfig, false, recreateBuilder)
		}
	}

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
	var ip = "public"
	if flag.GetBool(ctx, "flycast") || flag.GetBool(ctx, "attach") {
		ip = "private"
	} else if flag.GetBool(ctx, "no-public-ips") {
		ip = "none"
	}
	if appURL := appConfig.URL(); appURL != nil && ip == "public" {
		fmt.Fprintf(io.Out, "\nVisit your newly deployed app at %s\n", appURL)
	} else if ip == "private" {
		fmt.Fprintf(io.Out, "\nYour your newly deployed app is available in the organizations' private network under http://%s.flycast\n", appName)
	} else if ip == "none" {
		fmt.Fprintf(io.Out, "\nYour app is deployed but does not have a public or private IP address\n")
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
	cfg *appconfig.Config,
	app *fly.AppCompact,
	img *imgsrc.DeploymentImage,
) (err error) {
	var io = iostreams.FromContext(ctx)

	ctx, span := tracing.GetTracer().Start(ctx, "deploy_to_machines")
	defer span.End()
	// It's important to push appConfig into context because MachineDeployment will fetch it from there
	ctx = appconfig.WithConfig(ctx, cfg)

	startTime := time.Now()
	var status metrics.DeployStatusPayload

	metrics.Started(ctx, "deploy")
	// TODO: remove this once there is nothing upstream using it
	metrics.Started(ctx, "deploy_machines")

	defer func() {
		if err != nil {
			status.Error = err.Error()
		}
		status.TraceID = span.SpanContext().TraceID().String()
		status.Duration = time.Since(startTime)
		metrics.DeployStatus(ctx, status)
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

	excludeRegions := make(map[string]bool)
	for _, r := range flag.GetNonEmptyStringSlice(ctx, "exclude-regions") {
		excludeRegions[r] = true
	}

	onlyRegions := make(map[string]bool)
	for _, r := range flag.GetNonEmptyStringSlice(ctx, "regions") {
		onlyRegions[r] = true
	}

	excludeMachines := make(map[string]bool)
	for _, r := range flag.GetNonEmptyStringSlice(ctx, "exclude-machines") {
		excludeMachines[r] = true
	}

	onlyMachines := make(map[string]bool)
	for _, r := range flag.GetNonEmptyStringSlice(ctx, "only-machines") {
		onlyMachines[r] = true
	}

	processGroups := make(map[string]bool)
	for _, r := range flag.GetNonEmptyStringSlice(ctx, "process-groups") {
		processGroups[r] = true
	}

	// We default the flag to 0.33 so that --help can show the actual default value,
	// but internally we want to differentiate between the flag being specified and not.
	// We use 0.0 to denote unspecified, as that value is invalid for maxUnavailable.
	var maxUnavailable *float64 = nil
	if flag.IsSpecified(ctx, "max-unavailable") {
		maxUnavailable = fly.Pointer(flag.GetFloat64(ctx, "max-unavailable"))
		// Validation to ensure that 0.0 is *purely* the "unspecified" value
		if *maxUnavailable <= 0 {
			return fmt.Errorf("the value for --max-unavailable must be > 0")
		}
	}

	maxConcurrent := flag.GetInt(ctx, "max-concurrent")
	immediateMaxConcurrent := flag.GetInt(ctx, "immediate-max-concurrent")
	if maxConcurrent == defaultMaxConcurrent && immediateMaxConcurrent != defaultMaxConcurrent {
		maxConcurrent = immediateMaxConcurrent
	}

	status.AppName = app.Name
	status.OrgSlug = app.Organization.Slug
	status.Image = img.Tag
	status.Strategy = cfg.DeployStrategy()
	if flag.GetString(ctx, "strategy") != "" {
		status.Strategy = flag.GetString(ctx, "strategy")
	}

	if flag.IsSpecified(ctx, "primary-region") {
		status.PrimaryRegion = flag.GetString(ctx, "primary-region")
	} else {
		status.PrimaryRegion = cfg.PrimaryRegion
	}

	status.FlyctlVersion = buildinfo.Info().Version.String()

	retriesFlag := flag.GetString(ctx, "deploy-retries")
	deployRetries := 0

	switch retriesFlag {
	case "auto":
		ldClient := launchdarkly.ClientFromContext(ctx)
		retries := ldClient.GetFeatureFlagValue("deploy-retries", 0.0).(float64)
		deployRetries = int(retries)

	default:
		var invalidRetriesErr error = fmt.Errorf("--deploy-retries must be set to a positive integer, 0, or 'auto'")
		retries, err := strconv.Atoi(retriesFlag)
		if err != nil {
			return invalidRetriesErr
		}
		if retries < 0 {
			return invalidRetriesErr
		}

		span.SetAttributes(attribute.Int("set_deploy_retries", retries))
		deployRetries = retries
	}

	var ip = "public"
	if flag.GetBool(ctx, "flycast") || flag.GetBool(ctx, "attach") {
		ip = "private"
	} else if flag.GetBool(ctx, "no-public-ips") {
		ip = "none"
	}

	args := MachineDeploymentArgs{
		AppCompact:            app,
		DeploymentImage:       img.Tag,
		Strategy:              flag.GetString(ctx, "strategy"),
		EnvFromFlags:          flag.GetStringArray(ctx, "env"),
		PrimaryRegionFlag:     status.PrimaryRegion,
		SkipSmokeChecks:       flag.GetDetach(ctx) || !flag.GetBool(ctx, "smoke-checks"),
		SkipHealthChecks:      flag.GetDetach(ctx),
		SkipDNSChecks:         flag.GetDetach(ctx) || !flag.GetBool(ctx, "dns-checks"),
		SkipReleaseCommand:    flag.GetBool(ctx, "skip-release-command"),
		WaitTimeout:           waitTimeout,
		StopSignal:            flag.GetString(ctx, "signal"),
		ReleaseCmdTimeout:     releaseCmdTimeout,
		LeaseTimeout:          leaseTimeout,
		MaxUnavailable:        maxUnavailable,
		Guest:                 guest,
		IncreasedAvailability: flag.GetBool(ctx, "ha"),
		AllocIP:               ip,
		Org:                   app.Organization.Slug,
		UpdateOnly:            flag.GetBool(ctx, "update-only"),
		Files:                 files,
		ExcludeRegions:        excludeRegions,
		OnlyRegions:           onlyRegions,
		ExcludeMachines:       excludeMachines,
		OnlyMachines:          onlyMachines,
		MaxConcurrent:         maxConcurrent,
		VolumeInitialSize:     flag.GetInt(ctx, "volume-initial-size"),
		ProcessGroups:         processGroups,
		DeployRetries:         deployRetries,
		BuildID:               img.BuildID,
	}

	var path = flag.GetString(ctx, "export-manifest")
	switch {
	case path == "-":
		manifest := NewManifest(app.Name, cfg, args)

		return manifest.Encode(io.Out)

	case path != "":
		if !strings.HasSuffix(path, ".json") {
			path += ".json"
		}
		manifest := NewManifest(app.Name, cfg, args)

		if err = manifest.WriteToFile(path); err != nil {
			return err
		}
		fmt.Fprintf(io.Out, "Deploy manifest saved to %s\n", path)
		return nil
	}

	md, err := NewMachineDeployment(ctx, args)
	if err != nil {
		sentry.CaptureExceptionWithAppInfo(ctx, err, "deploy", app)
		return err
	}

	err = md.DeployMachinesApp(ctx)
	if err != nil {
		sentry.CaptureExceptionWithAppInfo(ctx, err, "deploy", app)
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

	// Always prefer the app name passed via --app
	if appName != "" {
		cfg.AppName = appName
	}

	err, extraInfo := cfg.Validate(ctx)
	if extraInfo != "" {
		fmt.Fprint(io.Out, extraInfo)
	}
	if err != nil {
		tracing.RecordError(span, err, "validate config")
		return nil, err
	}

	if cfg.Deploy != nil && cfg.Deploy.Strategy != "rolling" && cfg.Deploy.Strategy != "canary" && cfg.Deploy.MaxUnavailable != nil {
		if !config.FromContext(ctx).JSONOutput {
			fmt.Fprintf(io.Out, "Warning: max-unavailable set for non-rolling strategy '%s', ignoring\n", cfg.Deploy.Strategy)
		}
	}

	tb.Done("Verified app config")
	return cfg, nil
}
