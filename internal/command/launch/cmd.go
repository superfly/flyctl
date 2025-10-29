package launch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/validation"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
	"go.opentelemetry.io/otel/attribute"
)

func New() (cmd *cobra.Command) {
	const (
		long  = `Create and configure a new app from source code or a Docker image.  Options passed after double dashes ("--") will be passed to the language specific scanner/dockerfile generator.`
		short = `Create and configure a new app from source code or a Docker image`
	)

	cmd = command.New("launch", short, long, run, command.RequireSession, command.RequireUiex, command.LoadAppConfigIfPresent)
	cmd.Args = cobra.NoArgs

	flags := []flag.Flag{
		// Since launch can perform a deployment, we offer the full set of deployment flags for those using
		// the launch command in CI environments. We may want to rescind this decision down the line, because
		// the list of flags is long, but it follows from the precedent of already offering some deployment flags.
		// See a proposed 'flag grouping' feature in Viper that could help with DX: https://github.com/spf13/cobra/pull/1778
		deploy.CommonFlags,

		flag.Region(),
		flag.Org(),
		flag.NoDeploy(),
		flag.AppConfig(),
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
		flag.String{
			Name:        "from",
			Description: "A github repo URL to use as a template for the new app",
		},
		flag.String{
			Name:        "into",
			Description: "Destination directory for github repo specified with --from",
		},
		flag.Bool{
			Name:        "attach",
			Description: "Attach this new application to the current application",
		},
		flag.Bool{
			Name:        "manifest",
			Description: "Output the generated manifest to stdout",
			Hidden:      true,
		},
		flag.String{
			Name:        "from-manifest",
			Description: "Path to a manifest file for Launch ('-' reads from stdin)",
			Hidden:      true,
		},
		// legacy launch flags (deprecated)
		flag.Bool{
			Name:        "legacy",
			Description: "Use the legacy CLI interface (deprecated)",
			Hidden:      true,
		},
		flag.Bool{
			Name:        "reuse-app",
			Description: "Continue even if app name clashes with an existent app",
			Default:     false,
			Hidden:      true,
		},
		flag.Bool{
			Name:        "no-db",
			Description: "Skip automatically provisioning a database",
			Default:     false,
		},
		flag.Bool{
			Name:        "no-redis",
			Description: "Skip automatically provisioning a Redis instance",
			Default:     false,
		},
		flag.Bool{
			Name:        "no-object-storage",
			Description: "Skip automatically provisioning an object storage bucket",
			Default:     false,
		},
		flag.Bool{
			Name:        "no-github-workflow",
			Description: "Skip automatically provisioning a GitHub fly deploy workflow",
			Default:     false,
		},
		flag.Bool{
			Name:        "json",
			Description: "Generate configuration in JSON format",
		},
		flag.Bool{
			Name:        "yaml",
			Description: "Generate configuration in YAML format",
		},
		flag.Bool{
			Name:        "no-create",
			Description: "Do not create an app, only generate configuration files",
		},
		flag.String{
			Name:        "auto-stop",
			Description: "Automatically suspend the app after a period of inactivity. Valid values are 'off', 'stop', and 'suspend'",
			Default:     "stop",
		},
		flag.String{
			Name:        "command",
			Description: "The command to override the Docker CND.",
		},
		flag.StringSlice{
			Name:        "volume",
			Shorthand:   "v",
			Description: "Volume to mount, in the form of <volume_name>:/path/inside/machine[:<options>]",
		},
		flag.StringArray{
			Name:        "secret",
			Description: "Set of secrets in the form of NAME=VALUE pairs. Can be specified multiple times.",
		},
		flag.String{
			Name:        "db",
			Description: "Provision a Postgres database. Options: mpg (managed postgres), upg/legacy (unmanaged postgres), or true (default type)",
			NoOptDefVal: "true",
		},
	}

	flag.Add(cmd, flags...)

	cmd.AddCommand(NewPlan())

	return
}

func getManifestArgument(ctx context.Context) (*LaunchManifest, error) {
	path := flag.GetString(ctx, "from-manifest")
	if path == "" {
		return nil, nil
	}

	var jsonDecoder *json.Decoder
	if path != "-" {
		manifestJson, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		reader := bytes.NewReader(manifestJson)
		jsonDecoder = json.NewDecoder(reader)
	} else {
		// Read from stdin
		stdin := iostreams.FromContext(ctx).In
		jsonDecoder = json.NewDecoder(stdin)
	}

	var manifest LaunchManifest
	err := jsonDecoder.Decode(&manifest)
	if err != nil {
		return nil, err
	}
	return &manifest, nil
}

func setupFromTemplate(ctx context.Context) (context.Context, *appconfig.Config, error) {
	from := flag.GetString(ctx, "from")
	if from == "" {
		return ctx, nil, nil
	}

	into := flag.GetString(ctx, "into")

	if into == "" && flag.GetBool(ctx, "attach") {
		into = filepath.Join(".", "fly", "apps", filepath.Base(from))
	}

	if into == "" {
		into = "."
	} else {
		err := os.MkdirAll(into, 0755) // skipcq: GSC-G301
		if err != nil {
			return ctx, nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	var parentConfig *appconfig.Config

	entries, err := os.ReadDir(into)
	if err != nil {
		return ctx, nil, fmt.Errorf("failed to read directory: %w", err)
	}
	if len(entries) > 0 {
		return ctx, nil, errors.New("directory not empty, refusing to clone from git")
	}

	fmt.Printf("Launching from git repo %s\n", from)

	cmd := exec.Command("git", "clone", "--recurse-submodules", from, into)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return ctx, nil, err
	}

	if into != "." {
		err := os.Chdir(into)
		if err != nil {
			return ctx, nil, fmt.Errorf("failed to change directory: %w", err)
		}

		wd, err := os.Getwd()
		if err != nil {
			return ctx, nil, fmt.Errorf("failed determining working directory: %w", err)
		}

		ctx = state.WithWorkingDirectory(ctx, wd)
		parentConfig = appconfig.ConfigFromContext(ctx)
	}

	ctx = appconfig.WithConfig(ctx, nil)
	ctx, err = command.LoadAppConfigIfPresent(ctx)
	return ctx, parentConfig, err
}

func run(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)

	tp, err := tracing.InitTraceProviderWithoutApp(ctx)
	if err != nil {
		fmt.Fprintf(io.ErrOut, "failed to initialize tracing library: =%v", err)
		return err
	}

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		tp.Shutdown(shutdownCtx)
	}()

	ctx, span := tracing.CMDSpan(ctx, "cmd.launch")
	defer span.End()

	startTime := time.Now()
	var status metrics.LaunchStatusPayload
	metrics.Started(ctx, "launch")

	var state *launchState = nil

	if !flag.GetBool(ctx, "no-create") {
		defer func() {
			if err != nil {
				tracing.RecordError(span, err, "launch failed")
				status.Error = err.Error()

				if state != nil && state.sourceInfo != nil && state.sourceInfo.FailureCallback != nil {
					err = state.sourceInfo.FailureCallback(err)
				}
			}

			status.TraceID = span.SpanContext().TraceID().String()
			status.Duration = time.Since(startTime)
			metrics.LaunchStatus(ctx, status)
		}()
	}

	if err := warnLegacyBehavior(ctx); err != nil {
		return err
	}

	// Validate conflicting postgres flags
	if err := validatePostgresFlags(ctx); err != nil {
		return err
	}

	if err := validation.ValidateCompressionFlag(flag.GetString(ctx, "compression")); err != nil {
		return err
	}

	if err := validation.ValidateCompressionLevelFlag(flag.GetInt(ctx, "compression-level")); err != nil {
		return err
	}

	var (
		launchManifest *LaunchManifest
		cache          *planBuildCache
	)

	launchManifest, err = getManifestArgument(ctx)
	if err != nil {
		return err
	}

	// "--from" arg handling
	parentCtx := ctx
	ctx, parentConfig, err := setupFromTemplate(ctx)
	if err != nil {
		return err
	}

	incompleteLaunchManifest := false
	canEnterUi := !flag.GetBool(ctx, "manifest") && io.IsInteractive() && !env.IsCI()

	recoverableErrors := recoverableErrorBuilder{canEnterUi: canEnterUi}

	if launchManifest == nil {

		launchManifest, cache, err = buildManifest(ctx, parentConfig, &recoverableErrors)
		if err != nil {
			var recoverableErr recoverableInUiError
			if errors.As(err, &recoverableErr) && canEnterUi {
			} else {
				return err
			}
		}

		if flag.GetBool(ctx, "manifest") {
			var jsonEncoder *json.Encoder
			if manifestPath := flag.GetString(ctx, "manifest-path"); manifestPath != "" {
				file, err := os.OpenFile(manifestPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
				if err != nil {
					return err
				}
				defer file.Close()

				jsonEncoder = json.NewEncoder(file)
			} else {
				jsonEncoder = json.NewEncoder(io.Out)
			}
			jsonEncoder.SetIndent("", "  ")
			return jsonEncoder.Encode(launchManifest)
		}
	}

	// Override internal port if requested using --internal-port flag
	if n := flag.GetInt(ctx, "internal-port"); n > 0 {
		launchManifest.Plan.HttpServicePort = n
	}

	span.SetAttributes(attribute.String("app.name", launchManifest.Plan.AppName))

	status.AppName = launchManifest.Plan.AppName
	status.OrgSlug = launchManifest.Plan.OrgSlug
	status.Region = launchManifest.Plan.RegionCode
	status.HighAvailability = launchManifest.Plan.HighAvailability

	if len(launchManifest.Plan.Compute) > 0 {
		vm := launchManifest.Plan.Compute[0]
		status.VM.Size = vm.Size
		status.VM.Memory = vm.Memory
		status.VM.ProcessN = len(vm.Processes)
	}

	status.HasPostgres = launchManifest.Plan.Postgres.FlyPostgres != nil || launchManifest.Plan.Postgres.SupabasePostgres != nil || launchManifest.Plan.Postgres.ManagedPostgres != nil
	status.HasRedis = launchManifest.Plan.Redis.UpstashRedis != nil
	status.HasSentry = launchManifest.Plan.Sentry

	status.ScannerFamily = launchManifest.Plan.ScannerFamily
	status.FlyctlVersion = launchManifest.Plan.FlyctlVersion.String()

	state, err = stateFromManifest(ctx, *launchManifest, cache, &recoverableErrors)
	if err != nil {
		return err
	}

	summary, err := state.PlanSummary(ctx)
	if err != nil {
		return err
	}

	family := ""
	if state.sourceInfo != nil {
		family = state.sourceInfo.Family
	}

	planStep := plan.GetPlanStep(ctx)
	if planStep == "" {
		colorize := io.ColorScheme()

		// Get terminal width for responsive borders
		termWidth := io.TerminalWidth()
		if termWidth > 120 {
			termWidth = 120 // Cap at 120 for readability
		}
		border := strings.Repeat("═", termWidth)

		// Print top border
		fmt.Fprintln(io.Out)
		fmt.Fprintln(io.Out, border)
		fmt.Fprintln(io.Out)

		// Print ASCII art with purple = characters
		art := `


                       %%%%%%%
                  %#+++=.  .:+*+*#%%
               %#+=====:       :++=+#%
             %#+======-          .++==#%
            %*=======+.            .+==+%
           %#========+               *==+%
           #+========+              = *==*%
           #=========+           .. : .*=+#
           #=========*    ==      .:   *==*%
           #+========+-             :. +==*%
           #*=========+              :-*==*%         %@
           %#**+======+-           .-. #==*%      @%%%%@
           %%#++=======+:      .::.   :*=+%       @%
           %%%#*++=====++.            #=+#%      @%@
          %%  %%#*++++++*+.          **+#@       %@
         %%     @%#***+++*+         =*+#%%      %%
        @@        @%%##*****       :*+%@%%%    %%
        @@        @@@@%#***##     .**%%   %% %%%
        @      @@@@  @@@%%#**%   .+#%@     @%%
        @@@@@@@           @%#*#.:*%%
                            @%#**%@
                             @@%@@
                            %%#%%%
                          @#***+**%
                          %**+***#%
                          @@%#+*#%

`
		// Replace = with purple =
		artColored := strings.ReplaceAll(art, "=", colorize.Purple("="))
		fmt.Fprintln(io.Out, artColored)

		// Print header text
		fmt.Fprintf(io.Out, "%s\n\n", colorize.Bold(fmt.Sprintf("We're about to launch your %s on Fly.io. %s", familyToAppType(family), colorize.Purple("Here's what you're getting:"))))

		// Print summary table
		fmt.Fprintln(io.Out, summary)

		// Print bottom border
		fmt.Fprintln(io.Out, border)
		fmt.Fprintln(io.Out)
	}

	if errors := recoverableErrors.build(); errors != "" {

		fmt.Fprintf(io.ErrOut, "\n%s\n%s\n", aurora.Reverse(aurora.Red("The following problems must be fixed in the Launch UI:")), errors)
		incompleteLaunchManifest = true
	}

	// Check billing status and display appropriate message
	if planStep == "" {
		shouldContinue, err := checkBillingStatus(ctx, state)
		if err != nil {
			return err
		}
		if !shouldContinue {
			return errors.New("payment method required to continue")
		}
	}

	editInUi := false
	if !flag.GetBool(ctx, "yes") && planStep == "" {
		colorize := io.ColorScheme()
		if incompleteLaunchManifest {
			editInUi, err = prompt.ConfirmYes(ctx, "Would you like to continue in the web UI?")
		} else {
			editInUi, err = prompt.Confirm(ctx, colorize.Yellow("Do you want to tweak these settings before proceeding?"))
		}
		fmt.Fprintln(io.Out)

		if err != nil && !errors.Is(err, prompt.ErrNonInteractive) {
			return err
		}
	}

	if editInUi {
		err = state.EditInWebUi(ctx)
		if err != nil {
			return err
		}
	} else if incompleteLaunchManifest {
		// UI was required to reconcile launch issues, but user denied. Abort.
		return errors.New("launch can not continue with errors present")
	}

	err = state.Launch(ctx)
	if err != nil {
		return err
	}

	if flag.GetBool(ctx, "attach") && parentConfig != nil && !flag.GetBool(ctx, "no-create") {
		ctx, err = command.LoadAppConfigIfPresent(ctx)
		if err != nil {
			return err
		}

		config := appconfig.ConfigFromContext(ctx)

		exports := config.Experimental.Attached.Secrets.Export

		if len(exports) > 0 {
			flycast := config.AppName + ".flycast"
			for name, secret := range exports {
				exports[name] = strings.ReplaceAll(secret, "${FLYCAST_URL}", flycast)
			}

			// This might be duplicate work? Is there a saner place to build the client and stash it in the context?
			parentCtx, flapsClient, _, err := flapsutil.SetClient(parentCtx, nil, parentConfig.AppName)
			if err != nil {
				return fmt.Errorf("making client for %s: %w", parentConfig.AppName, err)
			}

			err = appsecrets.Update(parentCtx, flapsClient, parentConfig.AppName, exports, nil)
			if err != nil {
				return err
			}

			fmt.Fprintf(io.Out, "Set secrets on %s: %s\n", parentConfig.AppName, strings.Join(lo.Keys(exports), ", "))
		}
	}

	return nil
}

// familyToAppType returns a string that describes the app type based on the source info
// For example, "Dockerfile" apps would return "app" but a rails app would return "Rails app"
func familyToAppType(family string) string {
	switch family {
	case "Dockerfile":
		return "app"
	case "":
		return "app"
	}
	return fmt.Sprintf("%s app", family)
}

// warnLegacyBehavior warns the user if they are using a legacy flag
func warnLegacyBehavior(ctx context.Context) error {
	// TODO(Allison): We probably want to support re-configuring an existing app, but
	// that is different from the launch-into behavior of reuse-app, which basically just deployed.
	if flag.IsSpecified(ctx, "reuse-app") {
		return flyerr.GenericErr{
			Err:     "the --reuse-app flag is no longer supported. you are likely looking for 'fly deploy'",
			Suggest: "for now, you can use 'fly launch --legacy --reuse-app', but this will be removed in a future release",
		}
	}
	return nil
}

// validatePostgresFlags checks for conflicting postgres-related flags
func validatePostgresFlags(ctx context.Context) error {
	dbFlag := flag.GetString(ctx, "db")
	noDb := flag.GetBool(ctx, "no-db")

	// Normalize db flag values
	switch dbFlag {
	case "true", "1", "yes":
		dbFlag = "true"
	case "mpg", "managed":
		dbFlag = "mpg"
	case "upg", "unmanaged", "legacy":
		dbFlag = "upg"
	case "false", "0", "no", "":
		dbFlag = ""
	default:
		if dbFlag != "" {
			return flyerr.GenericErr{
				Err:     fmt.Sprintf("Invalid value '%s' for --db flag", dbFlag),
				Suggest: "Valid options: mpg (managed postgres), upg/legacy (unmanaged postgres), or true (default type)",
			}
		}
	}

	// Check if db flag conflicts with --no-db
	if dbFlag != "" && noDb {
		return flyerr.GenericErr{
			Err:     "Cannot specify both --db and --no-db",
			Suggest: "Remove either --db or --no-db",
		}
	}

	return nil
}

// checkBillingStatus checks the organization's billing status and displays appropriate messaging
// Returns (shouldContinue, error)
func checkBillingStatus(ctx context.Context, state *launchState) (bool, error) {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	// Skip billing check if running in non-interactive mode or CI
	if !io.IsInteractive() || env.IsCI() {
		return true, nil
	}

	// Fetch organization data including billing status
	org, err := state.orgCompact(ctx)
	if err != nil {
		// If we can't fetch org data, log the error but don't block the launch
		fmt.Fprintf(io.ErrOut, "Warning: Could not check billing status: %v\n", err)
		return true, nil
	}

	fmt.Fprintln(io.Out)

	switch org.BillingStatus {
	case gql.BillingStatusTrialActive:
		// User is on active free trial - celebrate!
		fmt.Fprintf(io.Out, "%s\n", colorize.Purple("✓ Your free trial has you covered - ship it! ✨"))

	case gql.BillingStatusSourceRequired, gql.BillingStatusTrialEnded:
		// User needs to add a payment method
		fmt.Fprintf(io.Out, "%s\n", colorize.Yellow("! You'll need to add a payment method in order to proceed."))
		fmt.Fprintln(io.Out)

		addPayment, err := prompt.Confirm(ctx, "Would you like to do this now?")
		if err != nil {
			return false, err
		}

		if addPayment {
			// Open billing dashboard URL
			billingURL := fmt.Sprintf("https://fly.io/dashboard/%s/billing", org.Slug)
			fmt.Fprintln(io.Out)
			fmt.Fprintf(io.Out, "Opening billing dashboard: %s\n", colorize.Bold(billingURL))
			fmt.Fprintln(io.Out)
			fmt.Fprintf(io.Out, "After adding a payment method, please run %s again.\n", colorize.Purple("'fly launch'"))

			// Try to open the URL in the browser
			if err := openBrowser(billingURL); err != nil {
				fmt.Fprintf(io.ErrOut, "Could not open browser automatically. Please visit: %s\n", billingURL)
			}

			return false, nil
		} else {
			return false, nil
		}

	case gql.BillingStatusCurrent, gql.BillingStatusPastDue, gql.BillingStatusDelinquent:
		// User has a payment method configured - say nothing per requirements
		// Just continue silently

	default:
		// Unknown billing status - log but don't block
		fmt.Fprintf(io.ErrOut, "Warning: Unknown billing status: %s\n", org.BillingStatus)
	}

	fmt.Fprintln(io.Out)
	return true, nil
}

// openBrowser attempts to open the given URL in the default browser
func openBrowser(url string) error {
	var cmd *exec.Cmd

	// Determine the OS and use the appropriate command
	// We can't use runtime.GOOS directly in exec context, so we check the environment
	if _, err := exec.LookPath("open"); err == nil {
		// macOS
		cmd = exec.Command("open", url)
	} else if _, err := exec.LookPath("xdg-open"); err == nil {
		// Linux
		cmd = exec.Command("xdg-open", url)
	} else if _, err := exec.LookPath("cmd"); err == nil {
		// Windows
		cmd = exec.Command("cmd", "/c", "start", url)
	} else {
		return fmt.Errorf("could not determine how to open browser")
	}

	return cmd.Start()
}
