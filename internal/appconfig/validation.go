package appconfig

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/shlex"
	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/sentry"
	"golang.org/x/exp/slices"
)

var ValidationError = errors.New("invalid app configuration")

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

	switch platformVersion {
	case MachinesPlatform:
		platErr, platExtra := cfg.ValidateForMachinesPlatform(ctx)
		return platErr, extra_info + platExtra
	case NomadPlatform:
		platErr, platExtra := cfg.ValidateForNomadPlatform(ctx)
		return platErr, extra_info + platExtra
	case "", DetachedPlatform:
		return nil, ""
	default:
		return fmt.Errorf("Unknown platform version '%s' for app '%s'", platformVersion, appName), extra_info
	}
}

func (cfg *Config) ValidateForNomadPlatform(ctx context.Context) (err error, extra_info string) {
	if extra, _ := cfg.validateBuildStrategies(); extra != "" {
		extra_info += extra
	}

	appName := NameFromContext(ctx)
	apiClient := client.FromContext(ctx).API()
	serverCfg, err := apiClient.ValidateConfig(ctx, appName, cfg.SanitizedDefinition())
	if err != nil {
		return err, extra_info
	}

	if _, haveHTTPService := cfg.RawDefinition["http_service"]; haveHTTPService {
		// TODO: eventually make this fail validation
		msg := fmt.Sprintf("%s the http_service section is ignored for Nomad apps", aurora.Yellow("WARN"))
		extra_info += msg + "\n"
		sentry.CaptureException(errors.New(msg))
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

func (cfg *Config) ValidateForMachinesPlatform(ctx context.Context) (err error, extra_info string) {
	validators := []func() (string, error){
		cfg.validateBuildStrategies,
		cfg.validateDeploySection,
		cfg.validateChecksSection,
		cfg.validateServicesSection,
		cfg.validateProcessesSection,
		cfg.validateMachineConversion,
	}

	for _, vFunc := range validators {
		info, vErr := vFunc()
		extra_info += info
		if vErr != nil {
			err = vErr
		}
	}

	if vErr := cfg.EnsureV2Config(); vErr != nil {
		err = vErr
	}

	if err != nil {
		extra_info += fmt.Sprintf("\n   %s%s\n", aurora.Red("✘"), err)
		return errors.New("App configuration is not valid"), extra_info
	}

	extra_info += fmt.Sprintf("%s Configuration is valid\n", aurora.Green("✓"))
	return nil, extra_info
}

func (cfg *Config) validateBuildStrategies() (extraInfo string, err error) {
	buildStrats := cfg.BuildStrategies()
	if len(buildStrats) > 1 {
		// TODO: validate that most users are not affected by this and/or fixing this, then make it fail validation
		msg := fmt.Sprintf("%s more than one build configuration found: [%s]", aurora.Yellow("WARN"), strings.Join(buildStrats, ", "))
		extraInfo += msg + "\n"
		sentry.CaptureException(errors.New(msg))
	}
	return
}

func (cfg *Config) validateDeploySection() (extraInfo string, err error) {
	if cfg.Deploy != nil {
		if _, vErr := shlex.Split(cfg.Deploy.ReleaseCommand); vErr != nil {
			extraInfo += fmt.Sprintf("Can't shell split release command: '%s'\n", cfg.Deploy.ReleaseCommand)
			err = ValidationError
		}
	}
	return
}

func (cfg *Config) validateChecksSection() (extraInfo string, err error) {
	for name, check := range cfg.Checks {
		if _, vErr := check.toMachineCheck(); vErr != nil {
			extraInfo += fmt.Sprintf("Can't process top level check '%s': %s\n", name, vErr)
			err = ValidationError
		}
	}
	return
}

func (cfg *Config) validateServicesSection() (extraInfo string, err error) {
	validGroupNames := cfg.ProcessNames()
	// The following is different than len(validGroupNames) because
	// it can be zero when there is no [processes] section
	processCount := len(cfg.Processes)

	for _, service := range cfg.AllServices() {
		switch {
		case len(service.Processes) == 0 && processCount > 0:
			extraInfo += fmt.Sprintf(
				"Service has no processes set but app has %d processes defined; update fly.toml to set processes for each service\n",
				processCount,
			)
			err = ValidationError
		default:
			for _, processName := range service.Processes {
				if !slices.Contains(validGroupNames, processName) {
					extraInfo += fmt.Sprintf(
						"Service specifies '%s' as one of its processes, but no processes are defined with that name; "+
							"update fly.toml [processes] to add '%s' process or remove it from service's processes list\n",
						processName, processName,
					)
					err = ValidationError
				}
			}
		}
	}
	return extraInfo, err
}

func (cfg *Config) validateProcessesSection() (extraInfo string, err error) {
	for processName, cmdStr := range cfg.Processes {
		if cmdStr == "" {
			continue
		}

		_, vErr := shlex.Split(cmdStr)
		if vErr != nil {
			extraInfo += fmt.Sprintf(
				"Could not parse command for '%s' process group; check [processes] section: %s\n",
				processName, vErr,
			)
			err = ValidationError
		}
	}

	return extraInfo, err
}

func (cfg *Config) validateMachineConversion() (extraInfo string, err error) {
	for _, name := range cfg.ProcessNames() {
		if _, vErr := cfg.ToMachineConfig(name); err != nil {
			extraInfo += fmt.Sprintf("Converting to machine in process group '%s' will fail because of: %s", name, vErr)
			err = ValidationError
		}
	}
	return
}
