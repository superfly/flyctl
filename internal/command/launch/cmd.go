package launch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
	"go.opentelemetry.io/otel/attribute"
)

func New() (cmd *cobra.Command) {
	const (
		long  = `Create and configure a new app from source code or a Docker image.  Options passed after double dashes ("--") will be passed to the language specific scanner/dockerfile generator.`
		short = `Create and configure a new app from source code or a Docker image`
	)

	cmd = command.New("launch", short, long, run, command.RequireSession, command.LoadAppConfigIfPresent)
	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		// Since launch can perform a deployment, we offer the full set of deployment flags for those using
		// the launch command in CI environments. We may want to rescind this decision down the line, because
		// the list of flags is long, but it follows from the precedent of already offering some deployment flags.
		// See a proposed 'flag grouping' feature in Viper that could help with DX: https://github.com/spf13/cobra/pull/1778
		deploy.CommonFlags,

		flag.Region(),
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
		flag.String{
			Name:        "config-file",
			Description: "Path to which the generated configuration file should be saved",
			Default:     "fly.toml",
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
			Name:        "json",
			Description: "Generate configuration in JSON format",
		},
		flag.Bool{
			Name:        "yaml",
			Description: "Generate configuration in YAML format",
		},
	)

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

func setupFromTemplate(ctx context.Context) (context.Context, error) {
	from := flag.GetString(ctx, "from")
	if from == "" {
		return ctx, nil
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		return ctx, fmt.Errorf("failed to read directory: %w", err)
	}
	if len(entries) > 0 {
		return ctx, errors.New("directory not empty, refusing to clone from git")
	}

	fmt.Printf("Launching from git repo %s\n", from)

	cmd := exec.Command("git", "clone", from, ".")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return ctx, err
	}

	ctx, err = command.LoadAppConfigIfPresent(ctx)
	return ctx, err
}

func run(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)

	tp, err := tracing.InitTraceProviderWithoutApp(ctx)
	if err != nil {
		fmt.Fprintf(io.ErrOut, "failed to initialize tracing library: =%v", err)
		return err
	}

	defer tp.Shutdown(ctx)

	ctx, span := tracing.CMDSpan(ctx, "cmd.launch")
	defer span.End()

	startTime := time.Now()
	var status metrics.LaunchStatusPayload
	metrics.Started(ctx, "launch")

	var state *launchState = nil

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

	if err := warnLegacyBehavior(ctx); err != nil {
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
	ctx, err = setupFromTemplate(ctx)
	if err != nil {
		return err
	}

	incompleteLaunchManifest := false
	canEnterUi := !flag.GetBool(ctx, "manifest") && io.IsInteractive() && !env.IsCI()

	recoverableErrors := recoverableErrorBuilder{canEnterUi: canEnterUi}

	if launchManifest == nil {

		launchManifest, cache, err = buildManifest(ctx, &recoverableErrors)
		if err != nil {
			var recoverableErr recoverableInUiError
			if errors.As(err, &recoverableErr) && canEnterUi {
			} else {
				return err
			}
		}

		if flag.GetBool(ctx, "manifest") {
			jsonEncoder := json.NewEncoder(io.Out)
			jsonEncoder.SetIndent("", "  ")
			return jsonEncoder.Encode(launchManifest)
		}
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

	status.HasPostgres = launchManifest.Plan.Postgres.FlyPostgres != nil
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

	fmt.Fprintf(
		io.Out,
		"We're about to launch your %s on Fly.io. Here's what you're getting:\n\n%s\n",
		familyToAppType(family),
		summary,
	)

	if errors := recoverableErrors.build(); errors != "" {

		fmt.Fprintf(io.ErrOut, "\n%s\n%s\n", aurora.Reverse(aurora.Red("The following problems must be fixed in the Launch UI:")), errors)
		incompleteLaunchManifest = true
	}

	editInUi := false
	if !flag.GetBool(ctx, "yes") {
		if incompleteLaunchManifest {
			editInUi, err = prompt.ConfirmYes(ctx, "Would you like to continue in the web UI?")
		} else {
			editInUi, err = prompt.Confirm(ctx, "Do you want to tweak these settings before proceeding?")
		}

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
