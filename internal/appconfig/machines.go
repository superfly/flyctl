package appconfig

import (
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

	machineConfig := &api.MachineConfig{
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

	machineConfig.Env["RELEASE_COMMAND"] = "1"
	if c.PrimaryRegion != "" {
		machineConfig.Env["PRIMARY_REGION"] = c.PrimaryRegion
	}

	return machineConfig, nil
}

//
func (c *Config) updateMachineConfig(src *api.MachineConfig) (*api.MachineConfig, error) {
	processGroup := c.DefaultProcessName()

	mConfig := &api.MachineConfig{}
	if src != nil {
		mConfig = helpers.Clone(src)
	}

	mConfig.Metrics = c.Metrics

	// Init
	cmd, err := c.InitCmd(processGroup)
	if err != nil {
		return nil, err
	}
	mConfig.Init.Cmd = cmd

	// Metadata
	if mConfig.Metadata == nil {
		mConfig.Metadata = map[string]string{}
	}
	mConfig.Metadata[api.MachineConfigMetadataKeyFlyPlatformVersion] = api.MachineFlyPlatformVersion2
	mConfig.Metadata[api.MachineConfigMetadataKeyFlyProcessGroup] = processGroup

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
			mConfig.Checks[checkName] = *machineCheck
		}
	}

	// Env
	mConfig.Env = lo.Assign(c.Env)
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
	if c.Mounts != nil {
		mConfig.Mounts = []api.MachineMount{{
			Path: c.Mounts.Destination,
			Name: c.Mounts.Source,
		}}
	}

	return mConfig, nil
}
