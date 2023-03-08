package config

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
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
	flag.Add(cmd, flag.App(), flag.AppConfig())
	return
}

func runValidate(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	apiClient := client.FromContext(ctx).API()

	cfg := appconfig.ConfigFromContext(ctx)
	if cfg == nil {
		return errors.New("App config file not found")
	}

	platformVersion := appconfig.NomadPlatform
	app, err := apiClient.GetAppBasic(ctx, appName)
	switch {
	case err == nil:
		platformVersion = app.PlatformVersion
	case strings.Contains(err.Error(), "Could not find App"):
		fmt.Fprintf(io.Out, "WARNING: Failed to fetch platform version: %s\n", err)
	default:
		return err
	}

	fmt.Fprintf(io.Out, "Validating %s (%s)\n", cfg.ConfigFilePath(), platformVersion)

	switch platformVersion {
	case appconfig.MachinesPlatform:
		err := cfg.EnsureV2Config()
		if err == nil {
			fmt.Fprintf(io.Out, "%s Configuration is valid\n", aurora.Green("✓"))
			return nil
		} else {
			fmt.Fprintf(io.Out, "\n   %s%s\n", aurora.Red("✘"), err)
			return errors.New("App configuration is not valid")
		}
	case appconfig.NomadPlatform:
		serverCfg, err := apiClient.ValidateConfig(ctx, appName, cfg.SanitizedDefinition())
		if err != nil {
			return err
		}

		if serverCfg.Valid {
			fmt.Fprintf(io.Out, "%s Configuration is valid\n", aurora.Green("✓"))
			return nil
		} else {
			fmt.Fprintf(io.Out, "\n")
			for _, errStr := range serverCfg.Errors {
				fmt.Fprintf(io.Out, "   %s%s\n", aurora.Red("✘"), errStr)
			}
			fmt.Fprintf(io.Out, "\n")
			return errors.New("App configuration is not valid")
		}
	default:
		return fmt.Errorf("Unknown platform version '%s' for app '%s'", platformVersion, appName)
	}
}
