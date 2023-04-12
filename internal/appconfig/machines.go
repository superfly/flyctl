package appconfig

import (
	"fmt"

	"github.com/google/shlex"
	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
)

func (c *Config) InitCmd(groupName string) ([]string, error) {
	if groupName == "" {
		groupName = c.DefaultProcessName()
	}
	cmdStr, ok := c.Processes[groupName]
	if !ok {
		return nil, nil
	}
	if cmdStr == "" {
		return nil, nil
	}

	cmd, err := shlex.Split(cmdStr)
	if err != nil {
		return nil, fmt.Errorf("could not parse command for %s process group: %w", groupName, err)
	}
	return cmd, nil
}

func (c *Config) AllServices() ([]Service, error) {
	var services []Service

	if c.HttpService != nil {
		services = append(services, *c.HttpService.ToService())
	}
	services = append(services, c.Services...)
	return services, nil
}

func (c *Config) DefaultMachineConfig() (*api.MachineConfig, error) {
	processGroup := c.DefaultProcessName()

	cmd, err := c.InitCmd(processGroup)
	if err != nil {
		return nil, err
	}

	machineConfig := &api.MachineConfig{
		Metrics: c.Metrics,
		Init:    api.MachineInit{Cmd: cmd},
		Metadata: map[string]string{
			api.MachineConfigMetadataKeyFlyPlatformVersion: api.MachineFlyPlatformVersion2,
			api.MachineConfigMetadataKeyFlyProcessGroup:    processGroup,
		},
		Env: lo.Assign(c.Env),
	}

	if services, err := c.AllServices(); err != nil {
		return nil, err
	} else if len(services) > 0 {
		machineConfig.Services = lo.Map(services, func(s Service, _ int) api.MachineService {
			return *s.toMachineService()
		})
	}

	if len(c.Checks) > 0 {
		machineConfig.Checks = map[string]api.MachineCheck{}
		for checkName, check := range c.Checks {
			machineCheck, err := check.toMachineCheck()
			if err != nil {
				return nil, err
			}
			machineConfig.Checks[checkName] = *machineCheck
		}
	}

	if c.PrimaryRegion != "" {
		machineConfig.Env["PRIMARY_REGION"] = c.PrimaryRegion
	}

	for _, s := range c.Statics {
		machineConfig.Statics = append(machineConfig.Statics, &api.Static{
			GuestPath: s.GuestPath,
			UrlPrefix: s.UrlPrefix,
		})
	}

	if c.Mounts != nil {
		machineConfig.Mounts = []api.MachineMount{{
			Path: c.Mounts.Destination,
			Name: c.Mounts.Source,
		}}
	}

	return machineConfig, nil
}

func (c *Config) ToMachineConfig(processGroup string) (*api.MachineConfig, error) {
	fc, err := c.Flatten(processGroup)
	if err != nil {
		return nil, err
	}
	return fc.DefaultMachineConfig()
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
