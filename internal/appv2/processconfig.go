package appv2

import (
	"fmt"

	"github.com/google/shlex"
	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
)

type ProcessConfig struct {
	Cmd      []string
	Services []api.MachineService
	Checks   map[string]api.MachineCheck
}

func (c *Config) GetProcessConfigs() (map[string]*ProcessConfig, error) {
	res := make(map[string]*ProcessConfig)
	processCount := len(c.Processes)
	configProcesses := lo.Assign(c.Processes)
	if processCount == 0 {
		configProcesses[api.MachineProcessGroupApp] = ""
	}
	defaultProcessName := lo.Keys(configProcesses)[0]

	for processName, cmdStr := range configProcesses {
		cmd := make([]string, 0)
		if cmdStr != "" {
			var err error
			cmd, err = shlex.Split(cmdStr)
			if err != nil {
				return nil, fmt.Errorf("could not parse command for %s process group: %w", processName, err)
			}
		}
		res[processName] = &ProcessConfig{
			Cmd:      cmd,
			Services: make([]api.MachineService, 0),
			Checks:   make(map[string]api.MachineCheck),
		}
	}

	for checkName, check := range c.Checks {
		machineCheck, err := check.toMachineCheck()
		if err != nil {
			return nil, err
		}
		for _, pc := range res {
			pc.Checks[checkName] = *machineCheck
		}
	}

	if c.HttpService != nil {
		if processCount > 1 {
			return nil, fmt.Errorf("http_service is not supported when more than one processes are defined "+
				"for an app, and this app has %d processes", processCount)
		}
		toUpdate := res[defaultProcessName]
		toUpdate.Services = append(toUpdate.Services, *c.HttpService.toMachineService())
	}

	for _, service := range c.Services {
		switch {
		case len(service.Processes) == 0 && processCount > 0:
			return nil, fmt.Errorf("error service has no processes set and app has %d processes defined;"+
				"update fly.toml to set processes for each service", processCount)
		case len(service.Processes) == 0 || processCount == 0:
			processName := defaultProcessName
			pc, present := res[processName]
			if processCount > 0 && !present {
				return nil, fmt.Errorf("error service specifies '%s' as one of its processes, but no "+
					"processes are defined with that name; update fly.toml [processes] to include a %s process", processName, processName)
			}
			pc.Services = append(pc.Services, *service.toMachineService())
		default:
			for _, processName := range service.Processes {
				pc, present := res[processName]
				if !present {
					return nil, fmt.Errorf("error service specifies '%s' as one of its processes, but no "+
						"processes are defined with that name; update fly.toml [processes] to include a %s process", processName, processName)
				}
				pc.Services = append(pc.Services, *service.toMachineService())
			}
		}
	}
	return res, nil
}
