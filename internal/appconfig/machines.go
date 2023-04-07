package appconfig

import (
	"fmt"

	"github.com/google/shlex"
	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
)

func (c *Config) ToMachineConfig(processGroup string) (*api.MachineConfig, error) {
	if processGroup == "" {
		processGroup = c.DefaultProcessName()
	}

	processConfigs, err := c.GetProcessConfigs()
	if err != nil {
		return nil, err
	}

	processConfig, ok := processConfigs[processGroup]
	if !ok {
		return nil, fmt.Errorf("unknown process group: %s", processGroup)
	}

	machineConfig := &api.MachineConfig{
		Metrics:  c.Metrics,
		Services: processConfig.Services,
		Checks:   processConfig.Checks,
		Init: api.MachineInit{
			Cmd: processConfig.Cmd,
		},
		Metadata: map[string]string{
			api.MachineConfigMetadataKeyFlyPlatformVersion: api.MachineFlyPlatformVersion2,
			api.MachineConfigMetadataKeyFlyProcessGroup:    processGroup,
		},
		Env: lo.Assign(c.Env),
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
