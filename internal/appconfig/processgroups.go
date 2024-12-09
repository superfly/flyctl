package appconfig

import (
	"fmt"
	"slices"
	"strings"

	"github.com/google/shlex"
	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
)

// ProcessNames lists each key of c.Processes, sorted lexicographically
// If c.Processes == nil, returns ["app"]
func (c *Config) ProcessNames() []string {
	if c == nil {
		return []string{fly.MachineProcessGroupApp}
	}
	switch names := lo.Keys(c.Processes); {
	case len(names) == 1:
		return names
	case len(names) > 1:
		slices.Sort(names)
		return names
	case c.defaultGroupName != "":
		return []string{c.defaultGroupName}
	default:
		return []string{fly.MachineProcessGroupApp}
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
		return fly.MachineProcessGroupApp
	}

	defaultGroupName := c.defaultGroupName
	if defaultGroupName == "" {
		defaultGroupName = fly.MachineProcessGroupApp
	}

	processNames := c.ProcessNames()
	if slices.Contains(processNames, defaultGroupName) {
		return defaultGroupName
	}
	return c.ProcessNames()[0]
}

// Checks if `toCheck` is a process group name that should target `target`
//
// Returns true if target == toCheck or if target is the default process name and toCheck is empty
func (c *Config) flattenGroupMatches(target, toCheck string) bool {
	if target == "" {
		target = c.DefaultProcessName()
	}
	switch {
	case toCheck == target:
		return true
	case toCheck == "" && target == c.DefaultProcessName():
		return true
	default:
		return false
	}
}

// Checks if any of the process group names in `toCheck` should target the group `target`
//
// Returns true if any of the groups in toCheck would return true for `flattenGroupMatches`,
// or if toCheck is empty, returns true if target is the default process name
func (c *Config) flattenGroupsMatch(target string, toCheck []string) bool {
	if len(toCheck) == 0 {
		return c.flattenGroupMatches(target, "")
	}
	return lo.SomeBy(toCheck, func(x string) bool {
		return c.flattenGroupMatches(target, x)
	})
}

// Flatten generates a machine config specific to a process_group.
//
// Only services, mounts, checks, metrics, files and restarts specific to the provided process group will be in the returned config.
func (c *Config) Flatten(groupName string) (*Config, error) {
	if err := c.SetMachinesPlatform(); err != nil {
		return nil, fmt.Errorf("can not flatten an invalid v2 application config: %w", err)
	}

	defaultGroupName := c.DefaultProcessName()
	if groupName == "" {
		groupName = defaultGroupName
	}
	matchesGroups := func(xs []string) bool {
		return c.flattenGroupsMatch(groupName, xs)
	}

	dst := helpers.Clone(c)
	dst.configFilePath = "--flatten--"
	dst.defaultGroupName = groupName

	// [processes]
	dst.Processes = lo.PickBy(dst.Processes, func(k, v string) bool {
		return dst.flattenGroupMatches(groupName, k)
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

	// [[restart]]
	dst.Restart = lo.Filter(dst.Restart, func(x Restart, _ int) bool {
		return matchesGroups(x.Processes)
	})
	for i := range dst.Restart {
		dst.Restart[i].Processes = []string{groupName}
	}

	// [[vm]]
	compute := dst.ComputeForGroup(groupName)

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

// ComputeForGroup finds the most specific VM compute requirements for this process group
// In reality there are only four valid cases:
//  1. No [[vm]] section
//  2. One [[vm]] section with `processes = [groupName]`
//  3. Previous case plus global [[compute]] without processes
//  4. Only a [[vm]] section without processes set which applies to all groups
func (c *Config) ComputeForGroup(groupName string) *Compute {
	if groupName == "" {
		groupName = c.DefaultProcessName()
	}

	compute := lo.MaxBy(
		// grab only the compute that matches or have no processes set
		lo.Filter(c.Compute, func(x *Compute, _ int) bool {
			return len(x.Processes) == 0 || c.flattenGroupsMatch(groupName, x.Processes)
		}),
		// Next find the most specific
		func(item *Compute, _ *Compute) bool {
			return slices.Contains(item.Processes, groupName)
		})
	return compute
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
