package appconfig

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/sentry"
)

func (cfg *Config) Validate(ctx context.Context) (err error, extra_info string) {
	appName := NameFromContext(ctx)
	apiClient := client.FromContext(ctx).API()

	if cfg == nil {
		return errors.New("App config file not found"), ""
	}

	extra_info = fmt.Sprintf("Validating %s\n", cfg.ConfigFilePath())

	platformVersion := cfg.platformVersion
	if platformVersion == "" {
		app, err := apiClient.GetAppBasic(ctx, appName)
		switch {
		case err == nil:
			platformVersion = app.PlatformVersion
			extra_info += fmt.Sprintf("Platform: %s\n", platformVersion)
		case strings.Contains(err.Error(), "Could not find App"):
			platformVersion = NomadPlatform
			extra_info += fmt.Sprintf("WARNING: Failed to fetch platform version: %s\n", err)
		default:
			return err, extra_info
		}
	} else {
		extra_info += fmt.Sprintf("Platform: %s\n", platformVersion)
	}

	buildStrats := cfg.BuildStrategies()
	if len(buildStrats) > 1 {
		// TODO: validate that most users are not affected by this and/or fixing this, then make it fail validation
		msg := fmt.Sprintf("%s more than one build configuration found: [%s]", aurora.Yellow("WARN"), strings.Join(buildStrats, ", "))
		extra_info += msg + "\n"
		sentry.CaptureException(errors.New(msg))
	}

	switch platformVersion {
	case MachinesPlatform:
		return cfg.ValidateForMachinesPlatform(ctx)
	case NomadPlatform:
		return cfg.ValidateForNomadPlatform(ctx)
	case "":
		return nil, ""
	default:
		return fmt.Errorf("Unknown platform version '%s' for app '%s'", platformVersion, appName), extra_info
	}

}

func (cfg *Config) ValidateForMachinesPlatform(ctx context.Context) (err error, extra_info string) {
	err = cfg.EnsureV2Config()
	if err == nil {
		extra_info += fmt.Sprintf("%s Configuration is valid\n", aurora.Green("✓"))
		return nil, extra_info
	} else {
		extra_info += fmt.Sprintf("\n   %s%s\n", aurora.Red("✘"), err)
		return errors.New("App configuration is not valid"), extra_info
	}
}

func (cfg *Config) ValidateForNomadPlatform(ctx context.Context) (err error, extra_info string) {
	appName := NameFromContext(ctx)
	apiClient := client.FromContext(ctx).API()
	serverCfg, err := apiClient.ValidateConfig(ctx, appName, cfg.SanitizedDefinition())
	if err != nil {
		return err, extra_info
	}

	if serverCfg.Valid {
		extra_info += fmt.Sprintf("%s Configuration is valid\n", aurora.Green("✓"))
		return nil, extra_info
	} else {
		for _, errStr := range serverCfg.Errors {
			extra_info += fmt.Sprintf("   %s%s\n", aurora.Red("✘"), errStr)
		}
		extra_info += "\n"
		return errors.New("App configuration is not valid"), extra_info
	}
}

func (cfg *Config) BuildStrategies() []string {
	strategies := []string{}

	if cfg == nil || cfg.Build == nil {
		return strategies
	}

	if cfg.Build.Image != "" {
		strategies = append(strategies, fmt.Sprintf("the \"%s\" docker image", cfg.Build.Image))
	}
	if cfg.Build.Builder != "" || len(cfg.Build.Buildpacks) > 0 {
		strategies = append(strategies, "a buildpack")
	}
	if cfg.Build.Dockerfile != "" || cfg.Build.DockerBuildTarget != "" {
		if cfg.Build.Dockerfile != "" {
			strategies = append(strategies, fmt.Sprintf("the \"%s\" dockerfile", cfg.Build.Dockerfile))
		} else {
			strategies = append(strategies, "a dockerfile")
		}
	}
	if cfg.Build.Builtin != "" {
		strategies = append(strategies, fmt.Sprintf("the \"%s\" builtin image", cfg.Build.Builtin))
	}

	return strategies
}
