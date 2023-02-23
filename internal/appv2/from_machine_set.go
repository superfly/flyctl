package appv2

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
)

func FromAppAndMachineSet(ctx context.Context, appCompact *api.AppCompact, machines machine.MachineSet) (*Config, string, error) {
	var (
		warnings                  []string
		io                        = iostreams.FromContext(ctx)
		colorize                  = io.ColorScheme()
		tomlCounter               = newFreqCounter[*machineConfigPair]()
		processGroups, warningMsg = processGroupsFromMachineSet(machines)
	)
	if warningMsg != "" {
		warnings = append(warnings, warningMsg)
	}
	for _, m := range machines.GetMachines() {
		appConfig, machineWarning := fromAppAndOneMachine(appCompact, m, processGroups)
		warnings = append(warnings, machineWarning)
		tomlString, err := appConfig.toTOMLString()
		if err != nil {
			warnings = append(warnings, warning("parse error", "error marshalling synthesized app config to fly.toml file for machine %s", m.Machine().ID))
		} else {
			tomlCounter.Capture(tomlString, &machineConfigPair{
				appConfig: appConfig,
				machine:   m,
			})
		}
	}
	if len(tomlCounter.items) == 0 {
		return nil, "", fmt.Errorf("could not create a fly.toml from any machines :-(\n%s", strings.Join(warnings, "\n"))
	}
	report := tomlCounter.Report()
	mostCommonConfig := report.mostCommonValues[0].appConfig
	if len(report.others) > 0 {
		for _, other := range report.otherValues {
			otherToml, err := other.appConfig.toTOMLString()
			if err == nil {
				warnings = append(warnings, warning("fly.toml", `Machine %s currently has a config that will change with the new fly.toml. This is what will change:
%s`, other.machine.Machine().ID, prettyDiff(otherToml, report.mostCommon, colorize)))
			}
		}
	}
	return mostCommonConfig, strings.Join(warnings, "\n"), nil
}

// FIXME: move this method somewhere else and share with machine.configCompare()
func prettyDiff(original, new string, colorize *iostreams.ColorScheme) string {
	diff := cmp.Diff(original, new)
	diffSlice := strings.Split(diff, "\n")
	var str string
	additionReg := regexp.MustCompile(`^\+.*`)
	deletionReg := regexp.MustCompile(`^\-.*`)
	for _, val := range diffSlice {
		vB := []byte(val)

		if additionReg.Match(vB) {
			str += colorize.Green(val) + "\n"
		} else if deletionReg.Match(vB) {
			str += colorize.Red(val) + "\n"
		} else {
			str += val + "\n"
		}
	}
	delim := "\"\"\""
	rx := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(delim) + `(.*?)` + regexp.QuoteMeta(delim))
	match := rx.FindStringSubmatch(str)
	if len(match) > 0 {
		return strings.Trim(match[1], "\n")
	}
	return ""
}

func fromAppAndOneMachine(appCompact *api.AppCompact, m machine.LeasableMachine, processGroups *processGroupInfo) (*Config, string) {
	var (
		warningMsg     string
		primaryRegion  string
		statics        []Static
		mounts         *Volume
		topLevelChecks map[string]*ToplevelCheck
	)
	for k, v := range m.Machine().Config.Env {
		if k == "PRIMARY_REGION" || k == "FLY_PRIMARY_REGION" {
			primaryRegion = v
			break
		}
	}
	for _, s := range m.Machine().Config.Statics {
		statics = append(statics, Static{
			GuestPath: s.GuestPath,
			UrlPrefix: s.UrlPrefix,
		})
	}
	if len(m.Machine().Config.Mounts) > 0 {
		mounts = &Volume{
			Destination: m.Machine().Config.Mounts[0].Path,
		}
	}
	if len(m.Machine().Config.Mounts) > 1 {
		var otherMounts string
		for _, mnt := range m.Machine().Config.Mounts {
			otherMounts += fmt.Sprintf("    %s (%s)\n", mnt.Path, mnt.Volume)
		}
		warningMsg += warning("mounts", `more than one mount attached to machine %s
fly.toml only supports one mount per machine at this time. These mounts will be removed on the next deploy:
%s
`, m.Machine().ID, otherMounts)
	}
	if len(m.Machine().Config.Checks) > 0 {
		topLevelChecks = make(map[string]*ToplevelCheck)
		for checkName, machineCheck := range m.Machine().Config.Checks {
			topLevelChecks[checkName] = topLevelCheckFromMachineCheck(machineCheck)
		}
	}
	return &Config{
		AppName:       appCompact.Name,
		KillSignal:    "SIGINT",
		KillTimeout:   5,
		PrimaryRegion: primaryRegion,
		Experimental:  nil,
		Build:         nil,
		Deploy:        nil,
		Env:           m.Machine().Config.Env,
		Metrics:       m.Machine().Config.Metrics,
		Statics:       statics,
		Mounts:        mounts,
		Processes:     processGroups.processes,
		Checks:        topLevelChecks,
		Services:      processGroups.services,
	}, warningMsg
}

func processGroupsFromMachineSet(ms machine.MachineSet) (*processGroupInfo, string) {
	var (
		warningMsg     string
		processGroups  = &processGroupInfo{}
		counter        = newFreqCounter[machine.LeasableMachine]()
		serviceCounter = newFreqCounter[machine.LeasableMachine]()
	)

	for _, m := range ms.GetMachines() {
		cmd := strings.Join(m.Machine().Config.Init.Cmd, " ")
		counter.Capture(cmd, m)
	}
	report := counter.Report()
	if report.mostCommon != "" {
		processGroups.processes = make(map[string]string)
		processGroups.processes[api.MachineProcessGroupApp] = report.mostCommon
	}
	if len(report.otherValues) > 0 {
		var otherMachineIds []string
		for _, m := range report.otherValues {
			otherMachineIds = append(otherMachineIds, m.Machine().ID)
		}
		otherCmds := ""
		for _, cmd := range report.otherValues {
			otherCmds += fmt.Sprintf("    %s\n", cmd)
		}
		warningMsg += warning("processes", fmt.Sprintf(`Found these additional commands on some machines. Consider adding process groups to your fly.toml and run machines with those process groups.
For more info please see: https://fly.io/docs/reference/configuration/#the-processes-section
Machine IDs that were not saved to fly.toml: %s
Commands they are running:
%s
`, strings.Join(otherMachineIds, ", "), otherCmds))
	}

	for _, m := range report.mostCommonValues {
		err := serviceCounter.Capture(m.Machine().Config.Services, m)
		if err != nil {
			terminal.Errorf("Failure processing machines: %v (skipping config for machine %s)\n", err, m.Machine().ID)
		}
	}
	serviceReport := serviceCounter.Report()
	processes := lo.Keys(processGroups.processes)
	for _, service := range serviceReport.mostCommonValues[0].Machine().Config.Services {
		processGroups.services = append(processGroups.services, *serviceFromMachineService(service, processes))
	}
	if len(serviceReport.otherValues) > 0 {
		var otherMachineIds []string
		for _, m := range serviceReport.otherValues {
			otherMachineIds = append(otherMachineIds, m.Machine().ID)
		}
		otherCmds := ""
		for _, cmd := range serviceReport.others {
			otherCmds += fmt.Sprintf("    %s\n", cmd)
		}
		warningMsg += warning("services", fmt.Sprintf(`Found different services on some other machines. Consider adding [[services]] block to fly.toml to support them.
For more info please see: https://fly.io/docs/reference/configuration/#the-services-sections
Machine IDs with different services: %s
`, strings.Join(otherMachineIds, ", ")))
	}

	return processGroups, warningMsg
}

type machineConfigPair struct {
	appConfig *Config
	machine   machine.LeasableMachine
}

type itemCount[T any] struct {
	count          int
	originalValues []T
}

type freqCounter[T any] struct {
	items map[string]*itemCount[T]
}

func newFreqCounter[T any]() *freqCounter[T] {
	return &freqCounter[T]{
		items: make(map[string]*itemCount[T]),
	}
}

func (c *freqCounter[T]) Capture(valueForComparison any, originalValue T) error {
	var key string
	switch valueForComparison := valueForComparison.(type) {
	case string:
		key = valueForComparison
	case []byte:
		key = string(valueForComparison)
	default:
		b, err := json.Marshal(valueForComparison)
		if err != nil {
			return fmt.Errorf("count json marshal %v error: %w", valueForComparison, err)
		}
		key = string(b)
	}
	if _, present := c.items[key]; !present {
		c.items[key] = &itemCount[T]{}
	}
	c.items[key].count += 1
	c.items[key].originalValues = append(c.items[key].originalValues, originalValue)
	return nil
}

func (c *freqCounter[T]) Report() *report[T] {
	var (
		highest int
		rep     = &report[T]{}
	)
	for val, item := range c.items {
		if item.count > highest {
			highest = item.count
			rep.mostCommon = val
			rep.mostCommonValues = item.originalValues
		}
	}
	for val, item := range c.items {
		if val != rep.mostCommon {
			rep.others = append(rep.others, val)
			rep.otherValues = append(rep.otherValues, item.originalValues...)
		}
	}
	return rep
}

type report[T any] struct {
	mostCommon       string
	mostCommonValues []T
	others           []string
	otherValues      []T
}

type processGroupInfo struct {
	processes map[string]string
	services  []Service
}

func warning(section, msg string, vals ...interface{}) string {
	w := fmt.Sprintf("WARNING [%s]: ", section)
	prefix := "\n"
	for range w {
		prefix += " "
	}
	for i, l := range strings.Split(fmt.Sprintf(msg, vals...), "\n") {
		if i > 0 {
			w += prefix
		}
		w += l
	}
	return w
}
