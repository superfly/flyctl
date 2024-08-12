package launch

import (
	"context"
	"encoding/json"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newGenerate() *cobra.Command {
	genDesc := "generates a launch manifest, including a config"
	cmd := command.New("generate", genDesc, genDesc, runGenerate,
		command.RequireAppName,
		command.LoadAppConfigIfPresent,
	)

	flag.Add(cmd,
		flag.App(),
		flag.Region(),
		flag.Org(),
		flag.AppConfig(),
		flag.String{
			Name:        "name",
			Description: `Name of the new app`,
		},
		// don't try to generate a name
		flag.Bool{
			Name:        "force-name",
			Description: "Force app name supplied by --name",
			Default:     false,
			Hidden:      true,
		},
		flag.Int{
			Name:        "internal-port",
			Description: "Set internal_port for all services in the generated fly.toml",
			Default:     -1,
		},
		flag.Bool{
			Name:        "ha",
			Description: "Create spare machines that increases app availability",
			Default:     false,
		},
		flag.String{
			Name:        "manifest-path",
			Shorthand:   "p",
			Description: "Path to write the manifest to",
			Default:     "manifest.json",
		},
		flag.Bool{
			Name:        "copy-config",
			Description: "Use the configuration file if present without prompting",
			Default:     false,
		},
	)

	// not that useful anywhere else yet
	cmd.Hidden = true

	return cmd
}

func runGenerate(ctx context.Context) error {
	ctx = context.WithValue(ctx, genContextKey{}, true)

	recoverableErrors := recoverableErrorBuilder{canEnterUi: false}
	launchManifest, planBuildCache, err := buildManifest(ctx, nil, &recoverableErrors)
	if err != nil {
		return err
	}

	updateConfig(launchManifest.Plan, nil, launchManifest.Config)

	if n := flag.GetInt(ctx, "internal-port"); n > 0 {
		launchManifest.Config.SetInternalPort(n)
	}

	manifestPath := flag.GetString(ctx, "manifest-path")

	file, err := os.Create(manifestPath)
	if err != nil {
		return err
	}
	defer file.Close()

	jsonEncoder := json.NewEncoder(file)
	jsonEncoder.SetIndent("", "  ")

	if err := jsonEncoder.Encode(launchManifest); err != nil {
		return err
	}

	state := &launchState{workingDir: ".", configPath: "fly.json", LaunchManifest: *launchManifest, env: map[string]string{}, planBuildCache: *planBuildCache, cache: map[string]interface{}{}}

	if err := state.satisfyScannerBeforeDb(ctx); err != nil {
		return err
	}

	if err = state.satisfyScannerAfterDb(ctx); err != nil {
		return err
	}

	if err = state.createDockerIgnore(ctx); err != nil {
		return err
	}

	if err = state.scannerSetAppconfig(ctx); err != nil {
		return err
	}

	return nil
}

type genContextKey struct{}

func isGenerate(ctx context.Context) bool {
	v, ok := ctx.Value(genContextKey{}).(bool)
	return ok && v
}
