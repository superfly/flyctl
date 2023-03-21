package appconfig

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/client"
)

func (cfg *Config) Validate(ctx context.Context) (err error, extraInfo string) {
	appName := NameFromContext(ctx)
	apiClient := client.FromContext(ctx).API()

	if cfg == nil {
		return errors.New("App config file not found"), ""
	}

	platformVersion := NomadPlatform
	extraInfo = fmt.Sprintf("Validating %s (%s)\n", cfg.ConfigFilePath(), platformVersion)

	app, err := apiClient.GetAppBasic(ctx, appName)
	switch {
	case err == nil:
		platformVersion = app.PlatformVersion
	case strings.Contains(err.Error(), "Could not find App"):
		extraInfo += fmt.Sprintf("WARNING: Failed to fetch platform version: %s\n", err)
	default:
		return err, extraInfo
	}

	switch platformVersion {
	case MachinesPlatform:
		return cfg.ValidateForMachinesPlatform(ctx)
	case NomadPlatform:
		return cfg.ValidateForNomadPlatform(ctx)
	case "":
		return nil, ""
	default:
		return fmt.Errorf("Unknown platform version '%s' for app '%s'", platformVersion, appName), extraInfo
	}
}

func (cfg *Config) ValidateForMachinesPlatform(ctx context.Context) (err error, extraInfo string) {
	err = cfg.EnsureV2Config()
	if err == nil {
		extraInfo += fmt.Sprintf("%s Configuration is valid\n", aurora.Green("✓"))
		return nil, extraInfo
	} else {
		extraInfo += fmt.Sprintf("\n   %s%s\n", aurora.Red("✘"), err)
		return errors.New("App configuration is not valid"), extraInfo
	}
}

func (cfg *Config) ValidateForNomadPlatform(ctx context.Context) (err error, extraInfo string) {
	appName := NameFromContext(ctx)
	apiClient := client.FromContext(ctx).API()
	serverCfg, err := apiClient.ValidateConfig(ctx, appName, cfg.SanitizedDefinition())
	if err != nil {
		return err, extraInfo
	}

	if serverCfg.Valid {
		extraInfo += fmt.Sprintf("%s Configuration is valid\n", aurora.Green("✓"))
		return nil, extraInfo
	} else {
		for _, errStr := range serverCfg.Errors {
			extraInfo += fmt.Sprintf("   %s%s\n", aurora.Red("✘"), errStr)
		}
		extraInfo += "\n"
		return errors.New("App configuration is not valid"), extraInfo
	}
}
