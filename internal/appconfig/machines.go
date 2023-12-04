package appconfig

import (
	"fmt"

	"github.com/docker/go-units"
	"github.com/google/shlex"
	"github.com/jinzhu/copier"
	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/machine"
)

func (c *Config) ToMachineConfig(processGroup string, src *api.MachineConfig) (*api.MachineConfig, error) {
	fc, err := c.Flatten(processGroup)
	if err != nil {
		return nil, err
	}
	return fc.updateMachineConfig(src)
}

func (c *Config) ToReleaseMachineConfig() (*api.MachineConfig, error) {
	releaseCmd, err := shlex.Split(c.Deploy.ReleaseCommand)
	if err != nil {
		return nil, err
	}

	mConfig := &api.MachineConfig{
		Init: api.MachineInit{
			Cmd:        releaseCmd,
			SwapSizeMB: c.SwapSizeMB,
		},
		Restart: api.MachineRestart{
			Policy: api.MachineRestartPolicyNo,
		},
		AutoDestroy: true,
		DNS: &api.DNSConfig{
			SkipRegistration: true,
		},
		Metadata: map[string]string{
			api.MachineConfigMetadataKeyFlyctlVersion:      buildinfo.Version().String(),
			api.MachineConfigMetadataKeyFlyPlatformVersion: api.MachineFlyPlatformVersion2,
			api.MachineConfigMetadataKeyFlyProcessGroup:    api.MachineProcessGroupFlyAppReleaseCommand,
		},
		Env: lo.Assign(c.Env),
	}

	if c.Experimental != nil {
		mConfig.Init.Entrypoint = c.Experimental.Entrypoint
	}

	mConfig.Env["RELEASE_COMMAND"] = "1"
	mConfig.Env["FLY_PROCESS_GROUP"] = api.MachineProcessGroupFlyAppReleaseCommand
	if c.PrimaryRegion != "" {
		mConfig.Env["PRIMARY_REGION"] = c.PrimaryRegion
	}

	// StopConfig
	c.tomachineSetStopConfig(mConfig)

	return mConfig, nil
}

func (c *Config) ToConsoleMachineConfig() (*api.MachineConfig, error) {
	mConfig := &api.MachineConfig{
		Init: api.MachineInit{
			// TODO: it would be better to configure init to run no
			// command at all. That way we don't rely on /bin/sleep
			// being available and working right. However, there's no
			// way to do that yet.
			Exec:       []string{"/bin/sleep", "inf"},
			SwapSizeMB: c.SwapSizeMB,
		},
		Restart: api.MachineRestart{
			Policy: api.MachineRestartPolicyNo,
		},
		AutoDestroy: true,
		DNS: &api.DNSConfig{
			SkipRegistration: true,
		},
		Metadata: map[string]string{
			api.MachineConfigMetadataKeyFlyctlVersion:      buildinfo.Version().String(),
			api.MachineConfigMetadataKeyFlyPlatformVersion: api.MachineFlyPlatformVersion2,
			api.MachineConfigMetadataKeyFlyProcessGroup:    api.MachineProcessGroupFlyAppConsole,
		},
		Env: lo.Assign(c.Env),
	}

	mConfig.Env["FLY_PROCESS_GROUP"] = api.MachineProcessGroupFlyAppConsole
	if c.PrimaryRegion != "" {
		mConfig.Env["PRIMARY_REGION"] = c.PrimaryRegion
	}

	return mConfig, nil
}

// updateMachineConfig applies configuration options from the optional MachineConfig passed in, then the base config, into a new MachineConfig
func (c *Config) updateMachineConfig(src *api.MachineConfig) (*api.MachineConfig, error) {
	// For flattened app configs there is only one proces name and it is the group it was flattened for
	processGroup := c.DefaultProcessName()

	mConfig := &api.MachineConfig{}
	if src != nil {
		mConfig = helpers.Clone(src)
	}

	// Metrics
	mConfig.Metrics = nil
	if len(c.Metrics) > 0 {
		mConfig.Metrics = c.Metrics[0].MachineMetrics
	}

	// Init
	cmd, err := c.InitCmd(processGroup)
	if err != nil {
		return nil, err
	}
	if c.Experimental != nil {
		if cmd == nil {
			cmd = c.Experimental.Cmd
		}
		mConfig.Init.Entrypoint = c.Experimental.Entrypoint
		mConfig.Init.Exec = c.Experimental.Exec
	} else {
		cmd = nil
		mConfig.Init.Entrypoint = nil
		mConfig.Init.Exec = nil
	}
	mConfig.Init.Cmd = cmd
	mConfig.Init.SwapSizeMB = c.SwapSizeMB

	// Metadata
	mConfig.Metadata = lo.Assign(mConfig.Metadata, map[string]string{
		api.MachineConfigMetadataKeyFlyctlVersion:      buildinfo.Version().String(),
		api.MachineConfigMetadataKeyFlyPlatformVersion: api.MachineFlyPlatformVersion2,
		api.MachineConfigMetadataKeyFlyProcessGroup:    processGroup,
	})

	// Services
	mConfig.Services = nil
	if services := c.AllServices(); len(services) > 0 {
		mConfig.Services = lo.Map(services, func(s Service, _ int) api.MachineService {
			return *s.toMachineService()
		})
	}

	// Checks
	mConfig.Checks = nil
	if len(c.Checks) > 0 {
		mConfig.Checks = map[string]api.MachineCheck{}
		for checkName, check := range c.Checks {
			machineCheck, err := check.toMachineCheck()
			if err != nil {
				return nil, err
			}
			if machineCheck.Port == nil {
				if c.HTTPService == nil {
					return nil, fmt.Errorf(
						"Check '%s' for process group '%s' has no port set and the group has no http_service to take it from",
						checkName, processGroup,
					)
				}
				machineCheck.Port = &c.HTTPService.InternalPort
			}
			mConfig.Checks[checkName] = *machineCheck
		}
	}

	// Env
	mConfig.Env = lo.Assign(c.Env)
	mConfig.Env["FLY_PROCESS_GROUP"] = processGroup
	if c.PrimaryRegion != "" {
		mConfig.Env["PRIMARY_REGION"] = c.PrimaryRegion
	}

	// Statics
	mConfig.Statics = nil
	for _, s := range c.Statics {
		mConfig.Statics = append(mConfig.Statics, &api.Static{
			GuestPath: s.GuestPath,
			UrlPrefix: s.UrlPrefix,
		})
	}

	// Mounts
	mConfig.Mounts = nil
	for _, m := range c.Mounts {
		var extendSizeIncrement, extendSizeLimit int

		if m.AutoExtendSizeIncrement != "" {
			// Ignore the error because invalid values are caught at config validation time
			extendSizeIncrement, _ = helpers.ParseSize(m.AutoExtendSizeIncrement, units.FromHumanSize, units.GB)
		}
		if m.AutoExtendSizeLimit != "" {
			// Ignore the error because invalid values are caught at config validation time
			extendSizeLimit, _ = helpers.ParseSize(m.AutoExtendSizeLimit, units.FromHumanSize, units.GB)
		}

		mConfig.Mounts = append(mConfig.Mounts, api.MachineMount{
			Path:                   m.Destination,
			Name:                   m.Source,
			ExtendThresholdPercent: m.AutoExtendSizeThreshold,
			AddSizeGb:              extendSizeIncrement,
			SizeGbLimit:            extendSizeLimit,
		})
	}

	// StopConfig
	c.tomachineSetStopConfig(mConfig)

	// Files
	mConfig.Files = nil
	machine.MergeFiles(mConfig, c.MergedFiles)

	// Guest
	if guest, err := c.toMachineGuest(); err != nil {
		return nil, err
	} else if guest != nil {
		// Only override machine's Guest if app config knows what to set
		mConfig.Guest = guest
	}

	return mConfig, nil
}

func (c *Config) tomachineSetStopConfig(mConfig *api.MachineConfig) error {
	mConfig.StopConfig = nil
	if c.KillSignal == nil && c.KillTimeout == nil {
		return nil
	}

	mConfig.StopConfig = &api.StopConfig{
		Timeout: c.KillTimeout,
		Signal:  c.KillSignal,
	}

	return nil
}

func (c *Config) toMachineGuest() (*api.MachineGuest, error) {
	// XXX: Don't be extra smart here, keep it backwards compatible with apps that don't have a [[compute]] section.
	// Think about apps that counts on `fly deploy` to respect whatever was set by `fly scale` or the --vm-* family flags.
	// It is important to return a `nil` guest when fly.toml doesn't contain a [[compute]] section for the process group.
	if len(c.Compute) == 0 {
		return nil, nil
	} else if len(c.Compute) > 2 {
		return nil, fmt.Errorf("2+ compute sections for group %s", c.DefaultProcessName())
	}

	// At most one compute after group flattening
	compute := c.Compute[0]

	size := api.DefaultVMSize
	switch {
	case compute.Size != "":
		size = compute.Size
	case compute.MachineGuest != nil && compute.MachineGuest.GPUKind != "":
		size = api.DefaultGPUVMSize
	}

	guest := &api.MachineGuest{}
	if err := guest.SetSize(size); err != nil {
		return nil, err
	}

	if c.HostDedicationID != "" {
		guest.HostDedicationID = c.HostDedicationID
	}

	if compute.Memory != "" {
		mb, err := helpers.ParseSize(compute.Memory, units.RAMInBytes, units.MiB)
		switch {
		case err != nil:
			return nil, err
		case mb == 0:
			return nil, fmt.Errorf("memory cannot be zero")
		default:
			guest.MemoryMB = mb
		}
	}

	if compute.MachineGuest != nil {
		opts := copier.Option{IgnoreEmpty: true, DeepCopy: true}
		err := copier.CopyWithOption(guest, compute.MachineGuest, opts)
		if err != nil {
			return nil, err
		}
	}

	return guest, nil
}
