package config

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/lib/appconfig"
	"github.com/superfly/flyctl/lib/command"
	"github.com/superfly/flyctl/lib/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newValidate() (cmd *cobra.Command) {
	const (
		short = "Validate an app's config file"
		long  = `Validates an application's config file against the Fly platform to
ensure it is correct and meaningful to the platform.`
	)
	cmd = command.New("validate", short, long, runValidate,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.NoArgs
	flag.Add(cmd, flag.App(), flag.AppConfig(), flag.Bool{
		Name:        "strict",
		Shorthand:   "s",
		Description: "Enable strict validation to check for unrecognized sections and keys",
		Default:     false,
	})
	return
}

func runValidate(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	cfg := appconfig.ConfigFromContext(ctx)
	strictMode := flag.GetBool(ctx, "strict")

	// if not found locally, try to get it from the remote app
	var err error
	if cfg == nil {
		appName := appconfig.NameFromContext(ctx)
		if appName == "" {
			return errors.New("app name is required")
		} else {
			cfg, err = appconfig.FromRemoteApp(ctx, appName)
			if err != nil {
				return err
			}
		}
	}

	var rawConfig map[string]any
	if strictMode {
		// Load config with raw data for strict validation
		rawConfig, err = appconfig.LoadConfigAsMap(cfg.ConfigFilePath())
		if err != nil {
			return fmt.Errorf("failed to load config for strict validation: %w", err)
		}
	}

	// Run standard validation
	if err = cfg.SetMachinesPlatform(); err != nil {
		return err
	}
	err, extraInfo := cfg.Validate(ctx)
	fmt.Fprintln(io.Out, extraInfo)

	// Run strict validation if enabled
	if strictMode {
		strictResult := appconfig.StrictValidate(rawConfig)

		if strictResult != nil && (len(strictResult.UnrecognizedSections) > 0 || len(strictResult.UnrecognizedKeys) > 0) {
			strictOutput := appconfig.FormatStrictValidationErrors(strictResult)
			if strictOutput != "" {
				fmt.Fprintf(io.Out, "\nStrict validation found unrecognised sections or keys:\n%s\n\n\n", strictOutput)
				// Return error to indicate validation failed
				if err == nil {
					err = errors.New("strict validation failed")
				}
			}
		}
	}

	return err
}
