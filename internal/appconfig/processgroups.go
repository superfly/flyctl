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

// ProcessNames lists each key of c.Processes, sorted lexicographically
// If c.Processes == nil, returns ["app"]
func (c *Config) ProcessNames() (names []string) {
	switch {
	case c == nil:
		return []string{api.MachineProcessGroupApp}
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

	switch {
	case len(names) == 1:
		return names
	case len(names) > 1:
		slices.Sort(names)
		return names
	case c.defaultGroupName != "":
		return []string{c.defaultGroupName}
	default:
		return []string{api.MachineProcessGroupApp}
	}
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
	if c == nil {
		return api.MachineProcessGroupApp
	}

	defaultGroupName := c.defaultGroupName
	if defaultGroupName == "" {
		defaultGroupName = api.MachineProcessGroupApp
	}

	processNames := c.ProcessNames()
	if slices.Contains(processNames, defaultGroupName) {
		return defaultGroupName
	}
	return c.ProcessNames()[0]
}

func (c *Config) Flatten(groupName string) (*Config, error) {
	if err := c.SetMachinesPlatform(); err != nil {
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
	dst.HTTPService = nil
	if c.HTTPService != nil && matchesGroups(c.HTTPService.Processes) {
		dst.HTTPService = c.HTTPService
	}

	// [[services]]
	dst.Services = lo.Filter(c.Services, func(s Service, _ int) bool {
		return matchesGroups(s.Processes)
	})

	return dst, nil
}

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
