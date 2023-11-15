package appconfig

import (
	"fmt"
	"slices"
	"strings"

	"github.com/google/shlex"
	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"
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

// Flatten generates a machine config specific to a process_group.
//
// Only services, mounts, checks, metrics & files specific to the provided progress group will be in the returned config.
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
		return lo.SomeBy(xs, matchesGroup)
	}

	dst := helpers.Clone(c)
	dst.platformVersion = c.platformVersion
	dst.configFilePath = "--flatten--"
	dst.defaultGroupName = groupName

	// [processes]
	dst.Processes = lo.PickBy(dst.Processes, func(k, v string) bool {
		return matchesGroup(k)
	})

	// [checks]
	dst.Checks = lo.PickBy(dst.Checks, func(_ string, check *ToplevelCheck) bool {
		return matchesGroups(check.Processes)
	})
	for i := range dst.Checks {
		dst.Checks[i].Processes = []string{groupName}
	}

	// [[http_service]]
	if dst.HTTPService != nil {
		if matchesGroups(dst.HTTPService.Processes) {
			dst.HTTPService.Processes = []string{groupName}
		} else {
			dst.HTTPService = nil
		}
	}

	// [[services]]
	dst.Services = lo.Filter(dst.Services, func(s Service, _ int) bool {
		return matchesGroups(s.Processes)
	})
	for i := range dst.Services {
		dst.Services[i].Processes = []string{groupName}
	}

	// [[Mounts]]
	dst.Mounts = lo.Filter(dst.Mounts, func(x Mount, _ int) bool {
		return matchesGroups(x.Processes)
	})
	for i := range dst.Mounts {
		dst.Mounts[i].Processes = []string{groupName}
	}

	// [[Files]]
	dst.Files = lo.Filter(dst.Files, func(x File, _ int) bool {
		return matchesGroups(x.Processes)
	})
	for i := range dst.Files {
		dst.Files[i].Processes = []string{groupName}
	}

	// [[metrics]]
	dst.Metrics = lo.Filter(dst.Metrics, func(x *Metrics, _ int) bool {
		return matchesGroups(x.Processes)
	})
	for i := range dst.Metrics {
		dst.Metrics[i].Processes = []string{groupName}
	}

	// [[vm]]
	// Find the most specific VM compute requirements for this process group
	// In reality there are only four valid cases:
	//   1. No [[vm]] section
	//   2. One [[vm]] section with `processes = [groupname]`
	//   3. Previous case plus global [[compute]] without processes
	//   4. Only a [[vm]] section without processes set which applies to all groups
	compute := lo.MaxBy(
		// grab only the compute that matches or have no processes set
		lo.Filter(dst.Compute, func(x *Compute, _ int) bool {
			return len(x.Processes) == 0 || matchesGroups(x.Processes)
		}),
		// Next find the most specific
		func(item *Compute, _ *Compute) bool {
			return slices.Contains(item.Processes, groupName)
		})

	dst.Compute = nil
	if compute != nil {
		// Sync top level host_dedication_id if set within compute
		if compute.MachineGuest != nil && compute.MachineGuest.HostDedicationID != "" {
			dst.HostDedicationID = compute.MachineGuest.HostDedicationID
		}
		compute.Processes = []string{groupName}
		dst.Compute = append(dst.Compute, compute)
	}

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
