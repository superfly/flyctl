package appconfig

import (
	"fmt"
	"strings"

	"github.com/google/shlex"
	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"
	"golang.org/x/exp/slices"
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
	defaultProcessName := c.DefaultProcessName()

	for processName, cmdStr := range configProcesses {
		var cmd []string
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

// ProcessNames lists each key of c.Processes, sorted lexicographically
// If c.Processes == nil, returns ["app"]
func (c *Config) ProcessNames() (names []string) {
	switch {
	case c == nil:
		break
	case c.platformVersion == MachinesPlatform:
		if len(c.Processes) != 0 {
			names = lo.Keys(c.Processes)
		}
	case c.platformVersion == "":
		fallthrough
	case c.platformVersion == DetachedPlatform:
		fallthrough
	case c.platformVersion == NomadPlatform:
		switch cast := c.RawDefinition["processes"].(type) {
		case map[string]any:
			if len(cast) != 0 {
				names = lo.Keys(cast)
			}
		case map[string]string:
			if len(cast) != 0 {
				names = lo.Keys(cast)
			}
		}
	}

	slices.Sort(names)
	if len(names) == 0 {
		names = []string{c.defaultGroupName}
	}
	return names
}

// FormatProcessNames formats the process group list like `['foo', 'bar']`
func (c *Config) FormatProcessNames() string {
	return "[" + strings.Join(lo.Map(c.ProcessNames(), func(s string, _ int) string {
		return "'" + s + "'"
	}), ", ") + "]"
}

// DefaultProcessName returns:
// * "app" when no processes are defined
// * "app" if present in the processes map
// * The first process name in ascending lexicographical order
func (c *Config) DefaultProcessName() string {
	processNames := c.ProcessNames()
	if slices.Contains(processNames, c.defaultGroupName) {
		return c.defaultGroupName
	}
	return c.ProcessNames()[0]
}

func (c *Config) Flatten(groupName string) (*Config, error) {
	if err := c.EnsureV2Config(); err != nil {
		return nil, fmt.Errorf("can not flatten an invalid v2 application config: %w", err)
	}

	defaultGroupName := c.DefaultProcessName()
	if groupName == "" {
		groupName = defaultGroupName
	}
	matchesGroup := func(x string) bool {
		switch {
		case x == groupName:
			return true
		case x == "" && groupName == defaultGroupName:
			return true
		default:
			return false
		}
	}
	matchesGroups := func(xs []string) bool {
		if len(xs) == 0 {
			return matchesGroup("")
		}
		for _, x := range xs {
			if matchesGroup(x) {
				return true
			}
		}
		return false
	}

	dst := helpers.Clone(c)
	dst.platformVersion = c.platformVersion
	dst.configFilePath = "--flatten--"
	dst.defaultGroupName = groupName

	// [processes]
	dst.Processes = nil
	for name, cmdStr := range c.Processes {
		if !matchesGroup(name) {
			continue
		}
		dst.Processes = map[string]string{dst.defaultGroupName: cmdStr}
		break
	}

	// [checks]
	dst.Checks = lo.PickBy(c.Checks, func(_ string, check *ToplevelCheck) bool {
		return matchesGroups(check.Processes)
	})

	// [[http_service]]
	dst.HttpService = nil
	if c.HttpService != nil && matchesGroups(c.HttpService.Processes) {
		dst.HttpService = c.HttpService
	}

	// [[services]]
	dst.Services = lo.Filter(c.Services, func(s Service, _ int) bool {
		return matchesGroups(s.Processes)
	})

	return dst, nil
}
