package appconfig

import (
	"fmt"

	"github.com/google/shlex"
	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"
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
			Cmd: releaseCmd,
		},
		Restart: api.MachineRestart{
			Policy: api.MachineRestartPolicyNo,
		},
		AutoDestroy: true,
		DNS: &api.DNSConfig{
			SkipRegistration: true,
		},
		Metadata: map[string]string{
			api.MachineConfigMetadataKeyFlyPlatformVersion: api.MachineFlyPlatformVersion2,
			api.MachineConfigMetadataKeyFlyProcessGroup:    api.MachineProcessGroupFlyAppReleaseCommand,
		},
		Env: lo.Assign(c.Env),
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
			Exec: []string{"/bin/sleep", "inf"},
		},
		Restart: api.MachineRestart{
			Policy: api.MachineRestartPolicyNo,
		},
		AutoDestroy: true,
		DNS: &api.DNSConfig{
			SkipRegistration: true,
		},
		Metadata: map[string]string{
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
	mConfig.Metrics = c.Metrics

	// Init
	cmd, err := c.InitCmd(processGroup)
	if err != nil {
		return nil, err
	}
	mConfig.Init.Cmd = cmd

	// Metadata
	mConfig.Metadata = lo.Assign(mConfig.Metadata, map[string]string{
		api.MachineConfigMetadataKeyFlyPlatformVersion: api.MachineFlyPlatformVersion2,
		api.MachineConfigMetadataKeyFlyProcessGroup:    processGroup,
	})

	// Services
	mConfig.Services = nil
	if services := c.AllServices(); len(services) > 0 {
		mConfig.Services = lo.Map(services, func(s Service, idx int) api.MachineService {
			svc := *s.toMachineService()
			if src != nil {
				svc.Autostop = src.Services[idx].Autostop
			}
			return svc
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
		mConfig.Mounts = append(mConfig.Mounts, api.MachineMount{
			Path: m.Destination,
			Name: m.Source,
		})
	}

	// StopConfig
	c.tomachineSetStopConfig(mConfig)

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
