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

	platformVersion := NomadPlatform
	extra_info = fmt.Sprintf("Validating %s (%s)\n", cfg.ConfigFilePath(), platformVersion)

	app, err := apiClient.GetAppBasic(ctx, appName)
	switch {
	case err == nil:
		platformVersion = app.PlatformVersion
	case strings.Contains(err.Error(), "Could not find App"):
		extra_info += fmt.Sprintf("WARNING: Failed to fetch platform version: %s\n", err)
	default:
		return err, extra_info
	}

	buildStrats := cfg.BuildStrategies()
	if len(buildStrats) > 1 {
		sentry.CaptureException(fmt.Errorf("More than one build configuration found: [%s]", strings.Join(buildStrats, ", ")))
	}

	switch platformVersion {
	case MachinesPlatform:
		err := cfg.EnsureV2Config()
		if err == nil {
			extra_info += fmt.Sprintf("%s Configuration is valid\n", aurora.Green("✓"))
			return nil, extra_info
		} else {
			extra_info += fmt.Sprintf("\n   %s%s\n", aurora.Red("✘"), err)
			return errors.New("App configuration is not valid"), extra_info
		}
	case NomadPlatform:
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
	case "":
		return nil, ""
	default:
		return fmt.Errorf("Unknown platform version '%s' for app '%s'", platformVersion, appName), extra_info
	}

}

func (cfg *Config) BuildStrategies() []string {
	strategies := []string{}

	if cfg.Build == nil {
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
