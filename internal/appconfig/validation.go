package appconfig

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/docker/go-units"
	"github.com/google/shlex"
	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/sentry"
)

var (
	ValidationError          = errors.New("invalid app configuration")
	MachinesDeployStrategies = []string{"canary", "rolling", "immediate", "bluegreen"}
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
		cfg.validateConsoleCommand,
		cfg.validateMounts,
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

func (cfg *Config) ValidateGroups(ctx context.Context, groups []string) (err error, extra_info string) {
	if len(groups) == 0 {
		return cfg.Validate(ctx)
	}
	var config *Config
	for _, group := range groups {
		config, err = cfg.Flatten(group)
		if err != nil {
			return
		}
		err, extra_info = config.Validate(ctx)
		if err != nil {
			return
		}
	}
	return
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
	if cfg.Deploy == nil {
		return
	}

	if _, vErr := shlex.Split(cfg.Deploy.ReleaseCommand); vErr != nil {
		extraInfo += fmt.Sprintf("Can't shell split release command: '%s'\n", cfg.Deploy.ReleaseCommand)
		err = ValidationError
	}

	if s := cfg.Deploy.Strategy; s != "" {
		if !slices.Contains(MachinesDeployStrategies, s) {
			extraInfo += fmt.Sprintf(
				"unsupported deployment strategy '%s'; Apps v2 supports the following strategies: %s", s,
				strings.Join(MachinesDeployStrategies, ", "),
			)
			err = ValidationError
		}

		if s == "canary" && len(cfg.Mounts) > 0 {
			extraInfo += "error canary deployment strategy is not supported when using mounted volumes"
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
		// minimum interval in flaps is set to 2 seconds.
		if check.Interval != nil && check.Interval.Duration.Seconds() < 2 {
			extraInfo += fmt.Sprintf("Check '%s' interval is too short: %s, minimum is 2 seconds\n", name, check.Interval.Duration)
			err = ValidationError
		}

		// max timeout in flaps in set to 60s
		if check.Timeout != nil && check.Timeout.Duration.Seconds() > 60 {
			extraInfo += fmt.Sprintf("Check '%s' timeout is too long: %s, maximum is 60 seconds\n", name, check.Timeout.Duration)
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

		for _, check := range service.TCPChecks {
			extraInfo += validateServiceCheckDurations(check.Interval, check.Timeout, check.GracePeriod, "TCP")
		}

		for _, check := range service.HTTPChecks {
			extraInfo += validateServiceCheckDurations(check.Interval, check.Timeout, check.GracePeriod, "HTTP")
		}
	}
	return extraInfo, err
}

func validateServiceCheckDurations(interval, timeout, gracePeriod *api.Duration, proto string) (extraInfo string) {
	extraInfo += validateSingleServiceCheckDuration(interval, false, proto, "an interval")
	extraInfo += validateSingleServiceCheckDuration(timeout, false, proto, "a timeout")
	extraInfo += validateSingleServiceCheckDuration(gracePeriod, true, proto, "a grace period")
	return
}

func validateSingleServiceCheckDuration(d *api.Duration, zeroOK bool, proto, description string) (extraInfo string) {
	switch {
	case d == nil:
		// Do nothing.
	case zeroOK && d.Duration != 0 && d.Duration < time.Second:
		extraInfo += fmt.Sprintf(
			"%s Service %s check has %s that is non-zero and less than 1 second (%v); this will be raised to 1 second\n",
			aurora.Yellow("WARN"), proto, description, d.Duration,
		)
	case !zeroOK && d.Duration < time.Second:
		extraInfo += fmt.Sprintf(
			"%s Service %s check has %s less than 1 second (%v); this will be raised to 1 second\n",
			aurora.Yellow("WARN"), proto, description, d.Duration,
		)
	case d.Duration > time.Minute:
		extraInfo += fmt.Sprintf(
			"%s Service %s check has %s greater than 1 minute (%v); this will be lowered to 1 minute\n",
			aurora.Yellow("WARN"), proto, description, d.Duration,
		)
	}
	return
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
		if _, vErr := cfg.ToMachineConfig(name, nil); err != nil {
			extraInfo += fmt.Sprintf("Converting to machine in process group '%s' will fail because of: %s", name, vErr)
			err = ValidationError
		}
	}
	return
}

func (cfg *Config) validateConsoleCommand() (extraInfo string, err error) {
	if _, vErr := shlex.Split(cfg.ConsoleCommand); vErr != nil {
		extraInfo += fmt.Sprintf("Can't shell split console command: '%s'\n", cfg.ConsoleCommand)
		err = ValidationError
	}
	return
}

func (cfg *Config) validateMounts() (extraInfo string, err error) {
	if cfg.configFilePath == "--flatten--" && len(cfg.Mounts) > 1 {
		extraInfo += fmt.Sprintf("group '%s' has more than one [[mounts]] section defined\n", cfg.defaultGroupName)
		err = ValidationError
	}

	for _, m := range cfg.Mounts {
		if m.InitialSize != "" {
			v, vErr := helpers.ParseSize(m.InitialSize, units.FromHumanSize, units.GB)
			switch {
			case vErr != nil:
				extraInfo += fmt.Sprintf("mount '%s' with initial_size '%s' will fail because of: %s\n", m.Source, m.InitialSize, vErr)
				err = ValidationError
			case v < 1:
				extraInfo += fmt.Sprintf("mount '%s' has an initial_size '%s' value which is smaller than 1GB\n", m.Source, m.InitialSize)
				err = ValidationError
			}
		}
	}
	return
}
