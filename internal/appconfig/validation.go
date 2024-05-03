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
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/sentry"
)

var (
	ValidationError          = errors.New("invalid app configuration")
	MachinesDeployStrategies = []string{"canary", "rolling", "immediate", "bluegreen"}
)

func (cfg *Config) Validate(ctx context.Context) (err error, extra_info string) {
	if cfg == nil {
		return errors.New("App config file not found"), ""
	}

	validators := []func() (string, error){
		cfg.validateBuildStrategies,
		cfg.validateDeploySection,
		cfg.validateChecksSection,
		cfg.validateServicesSection,
		cfg.validateProcessesSection,
		cfg.validateMachineConversion,
		cfg.validateConsoleCommand,
		cfg.validateMounts,
		cfg.validateRestartPolicy,
	}

	extra_info = fmt.Sprintf("Validating %s\n", cfg.ConfigFilePath())

	for _, vFunc := range validators {
		info, vErr := vFunc()
		extra_info += info
		if vErr != nil {
			err = vErr
		}
	}

	if cfg.v2UnmarshalError != nil {
		err = cfg.v2UnmarshalError
	}

	if err != nil {
		extra_info += fmt.Sprintf("\n   %s%s\n", aurora.Red("✘"), err)
		return errors.New("App configuration is not valid"), extra_info
	}

	extra_info += fmt.Sprintf("%s Configuration is valid\n", aurora.Green("✓"))
	return nil, extra_info
}

func (cfg *Config) ValidateGroups(ctx context.Context, groups []string) (err error, extraInfo string) {
	if len(groups) == 0 {
		return cfg.Validate(ctx)
	}
	var config *Config
	for _, group := range groups {
		config, err = cfg.Flatten(group)
		if err != nil {
			return
		}
		err, extraInfo = config.Validate(ctx)
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

		if len(service.Ports) == 0 {
			// XXX: Warn about services without ports instead of hard failing so users have time to
			//      fix fly.toml configuration -- 2024-01-15
			extraInfo += fmt.Sprintf(
				"WARNING: Service must expose at least one port. Add a [[services.ports]] section to fly.toml; " +
					"Check docs at https://fly.io/docs/reference/configuration/#services-ports \n " +
					"Validation for _services without ports_ will hard fail after February 15, 2024.",
			)
			//err = ValidationError
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

func validateServiceCheckDurations(interval, timeout, gracePeriod *fly.Duration, proto string) (extraInfo string) {
	extraInfo += validateSingleServiceCheckDuration(interval, false, proto, "an interval")
	extraInfo += validateSingleServiceCheckDuration(timeout, false, proto, "a timeout")
	extraInfo += validateSingleServiceCheckDuration(gracePeriod, true, proto, "a grace period")
	return
}

func validateSingleServiceCheckDuration(d *fly.Duration, zeroOK bool, proto, description string) (extraInfo string) {
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

		if m.SnapshotRetention != nil && (*m.SnapshotRetention < 1 || *m.SnapshotRetention > 60) {
			extraInfo += fmt.Sprintf("mount '%s' has a snapshot_retention value which is not between 1 and 60 days inclusive\n", m.Source)
			err = ValidationError
		}

		var autoExtendSizeIncrement, autoExtendSizeLimit int
		var vErr error
		if m.AutoExtendSizeIncrement != "" {
			autoExtendSizeIncrement, vErr = helpers.ParseSize(m.AutoExtendSizeIncrement, units.FromHumanSize, units.GB)
			switch {
			case vErr != nil:
				extraInfo += fmt.Sprintf("mount '%s' with auto_extend_size_increment '%s' will fail because of: %s\n", m.Source, m.AutoExtendSizeIncrement, vErr)
				err = ValidationError
			case autoExtendSizeIncrement < 1:
				extraInfo += fmt.Sprintf("mount '%s' has an auto_extend_size_increment '%s' value which is smaller than 1GB\n", m.Source, m.AutoExtendSizeIncrement)
				err = ValidationError
			}
		}
		if m.AutoExtendSizeLimit != "" {
			autoExtendSizeLimit, vErr = helpers.ParseSize(m.AutoExtendSizeLimit, units.FromHumanSize, units.GB)
			switch {
			case vErr != nil:
				extraInfo += fmt.Sprintf("mount '%s' with auto_extend_size_limit '%s' will fail because of: %s\n", m.Source, m.AutoExtendSizeLimit, vErr)
				err = ValidationError
			case autoExtendSizeLimit < 1:
				extraInfo += fmt.Sprintf("mount '%s' has an auto_extend_size_limit '%s' value which is smaller than 1GB\n", m.Source, m.AutoExtendSizeLimit)
				err = ValidationError
			}
		}

		if m.AutoExtendSizeThreshold != 0 || autoExtendSizeIncrement != 0 || autoExtendSizeLimit != 0 {
			if m.AutoExtendSizeThreshold != 0 && autoExtendSizeIncrement == 0 && autoExtendSizeLimit == 0 {
				extraInfo += fmt.Sprintf("mount '%s' auto_extend_size_threshold, auto_extend_size_increment and auto_extend_size_limit must be all defined or none\n", m.Source)
				err = ValidationError
			}
			if m.AutoExtendSizeThreshold < 50 || m.AutoExtendSizeThreshold > 99 {
				extraInfo += fmt.Sprintf("mount '%s' auto_extend_size_threshold must be between 50 and 99\n", m.Source)
				err = ValidationError
			}
			if autoExtendSizeIncrement < 1 || autoExtendSizeIncrement > 100 {
				extraInfo += fmt.Sprintf("mount '%s' auto_extend_size_increment must be between 1GB and 100GB\n", m.Source)
				err = ValidationError
			}
			if autoExtendSizeLimit != 0 && (autoExtendSizeLimit < 1 || autoExtendSizeLimit > 500) {
				extraInfo += fmt.Sprintf("mount '%s' auto_extend_size_limit must be between 1GB and 500GB\n", m.Source)
				err = ValidationError
			}
		}
	}
	return
}

func (cfg *Config) validateRestartPolicy() (extraInfo string, err error) {
	if cfg.Restart == nil {
		return
	}

	for _, restart := range cfg.Restart {
		validGroupNames := cfg.ProcessNames()

		// first make sure restart.Processes matches a valid process name.
		for _, processName := range restart.Processes {
			if !slices.Contains(validGroupNames, processName) {
				extraInfo += fmt.Sprintf("Restart policy specifies '%s' as one of its processes, but no processes are defined with that name; "+
					"update fly.toml [processes] to add '%s' process or remove it from restart policy's processes list\n",
					processName, processName,
				)
				err = ValidationError
			}
		}

		_, vErr := parseRestartPolicy(restart.Policy)
		if vErr != nil {
			extraInfo += fmt.Sprintf("%s\n", vErr)
			err = ValidationError
		}
	}

	return
}
