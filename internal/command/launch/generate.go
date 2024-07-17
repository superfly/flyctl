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
	)

	// not that useful anywhere else yet
	cmd.Hidden = true

	return cmd
}

func runGenerate(ctx context.Context) error {
	ctx = context.WithValue(ctx, genContextKey{}, true)

	recoverableErrors := recoverableErrorBuilder{canEnterUi: false}
	launchManifest, _, err := buildManifest(ctx, &recoverableErrors)
	if err != nil {
		return err
	}

	updateConfig(launchManifest.Plan, nil, launchManifest.Config)

	file, err := os.Create(flag.GetString(ctx, "manifest-path"))
	if err != nil {
		return err
	}
	defer file.Close()

	jsonEncoder := json.NewEncoder(file)
	jsonEncoder.SetIndent("", "  ")

	return jsonEncoder.Encode(launchManifest)
}

type genContextKey struct{}

func isGenerate(ctx context.Context) bool {
	v, ok := ctx.Value(genContextKey{}).(bool)
	return ok && v
}