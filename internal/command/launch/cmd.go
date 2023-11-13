package launch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/command/launch/legacy"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func New() (cmd *cobra.Command) {
	const (
		long  = `Create and configure a new app from source code or a Docker image`
		short = long
	)

	cmd = command.New("launch", short, long, run, command.RequireSession, command.LoadAppConfigIfPresent)
	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		// Since launch can perform a deployment, we offer the full set of deployment flags for those using
		// the launch command in CI environments. We may want to rescind this decision down the line, because
		// the list of flags is long, but it follows from the precedent of already offering some deployment flags.
		// See a proposed 'flag grouping' feature in Viper that could help with DX: https://github.com/spf13/cobra/pull/1778
		deploy.CommonFlags,

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
		flag.Bool{
			Name:        "copy-config",
			Description: "Use the configuration file if present without prompting",
			Default:     false,
		},
		flag.Bool{
			Name:        "reuse-app",
			Description: "Continue even if app name clashes with an existent app",
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
		// Launch V2
		flag.Bool{
			Name:        "no-ui",
			Description: "Use the legacy CLI interface (deprecated)",
			Hidden:      true,
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

func run(ctx context.Context) (err error) {

	if flag.GetBool(ctx, "no-ui") {
		return legacy.Run(ctx)
	}

	io := iostreams.FromContext(ctx)

	if err := warnLegacyBehavior(ctx); err != nil {
		return err
	}

	// TODO: Metrics

	var (
		launchManifest *LaunchManifest
		cache          *planBuildCache
	)

	launchManifest, err = getManifestArgument(ctx)
	if err != nil {
		return err
	}

	incompleteLaunchManifest := false
	canEnterUi := !flag.GetBool(ctx, "manifest")

	if launchManifest == nil {

		launchManifest, cache, err = buildManifest(ctx, canEnterUi)
		if err != nil {
			var recoverableErr recoverableInUiError
			if errors.As(err, &recoverableErr) && canEnterUi {
				fmt.Fprintln(io.ErrOut, "The following problems must be fixed in the Launch UI:")
				fmt.Fprintln(io.ErrOut, recoverableErr.Error())
				incompleteLaunchManifest = true
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

	state, err := stateFromManifest(ctx, *launchManifest, cache)
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

	editInUi := false
	if io.IsInteractive() {
		prompt := &survey.Confirm{
			Message: lo.Ternary(
				incompleteLaunchManifest,
				"Would you like to continue in the web UI?",
				"Do you want to tweak these settings before proceeding?",
			),
		}
		err = survey.AskOne(prompt, &editInUi)
		if err != nil {
			// TODO(allison): This should probably not just return the error
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
		return errors.New("the --reuse-app flag is no longer supported. you are likely looking for 'fly deploy'")
	}
	return nil
}
