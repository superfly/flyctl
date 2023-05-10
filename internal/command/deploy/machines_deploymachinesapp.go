package deploy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/helpers"
	machcmd "github.com/superfly/flyctl/internal/command/machine"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

type ProcessGroupsDiff struct {
	machinesToRemove      []machine.LeasableMachine
	groupsToRemove        map[string]int
	groupsNeedingMachines map[string]bool
}

func (md *machineDeployment) DeployMachinesApp(ctx context.Context) error {
	ctx = flaps.NewContext(ctx, md.flapsClient)

	if err := md.updateReleaseInBackend(ctx, "running"); err != nil {
		return fmt.Errorf("failed to set release status to 'running': %w", err)
	}

	var err error
	if md.restartOnly {
		err = md.restartMachinesApp(ctx)
	} else {
		err = md.deployMachinesApp(ctx)
	}

	var status string
	switch {
	case err == nil:
		status = "complete"
	case errors.Is(err, context.Canceled):
		// Provide an extra second to try to update the release status.
		status = "interrupted"
		var cancel func()
		ctx, cancel = context.WithTimeout(context.Background(), time.Second)
		defer cancel()
	default:
		status = "failed"
	}

	if updateErr := md.updateReleaseInBackend(ctx, status); updateErr != nil {
		if err == nil {
			err = fmt.Errorf("failed to set final release status: %w", updateErr)
		} else {
			terminal.Warnf("failed to set final release status after deployment failure: %v\n", updateErr)
		}
	}
	return err
}

// restartMachinesApp only restarts existing machines but updates their release metadata
func (md *machineDeployment) restartMachinesApp(ctx context.Context) error {
	if err := md.machineSet.AcquireLeases(ctx, md.leaseTimeout); err != nil {
		return err
	}
	defer md.machineSet.ReleaseLeases(ctx) // skipcq: GO-S2307
	md.machineSet.StartBackgroundLeaseRefresh(ctx, md.leaseTimeout, md.leaseDelayBetween)

	machineUpdateEntries := lo.Map(md.machineSet.GetMachines(), func(lm machine.LeasableMachine, _ int) *machineUpdateEntry {
		return &machineUpdateEntry{leasableMachine: lm, launchInput: md.launchInputForRestart(lm.Machine())}
	})

	return md.updateExistingMachines(ctx, machineUpdateEntries)
}

// deployMachinesApp executes the following flow:
//   - Run release command
//   - Remove spare machines from removed groups
//   - Launch new machines on new groups
//   - Update existing machines
func (md *machineDeployment) deployMachinesApp(ctx context.Context) error {
	if err := md.runReleaseCommand(ctx); err != nil {
		return fmt.Errorf("release command failed - aborting deployment. %w", err)
	}

	if err := md.machineSet.AcquireLeases(ctx, md.leaseTimeout); err != nil {
		return err
	}
	defer md.machineSet.ReleaseLeases(ctx) // skipcq: GO-S2307
	md.machineSet.StartBackgroundLeaseRefresh(ctx, md.leaseTimeout, md.leaseDelayBetween)

	processGroupMachineDiff := md.resolveProcessGroupChanges()
	md.warnAboutProcessGroupChanges(ctx, processGroupMachineDiff)

	if len(processGroupMachineDiff.machinesToRemove) > 0 {
		// Destroy machines that don't fit the current process groups
		if err := md.machineSet.RemoveMachines(ctx, processGroupMachineDiff.machinesToRemove); err != nil {
			return err
		}
		for _, mach := range processGroupMachineDiff.machinesToRemove {
			if err := machcmd.Destroy(ctx, md.app, mach.Machine(), true); err != nil {
				return err
			}
		}
	}

	// Create machines for new process groups
	if total := len(processGroupMachineDiff.groupsNeedingMachines); total > 0 {
		groupsWithAutostopEnabled := make(map[string]bool)

		for idx, name := range maps.Keys(processGroupMachineDiff.groupsNeedingMachines) {
			fmt.Fprintf(md.io.Out, "No machines in group %s, launching one new machine\n", md.colorize.Bold(name))
			machineID, err := md.spawnMachineInGroup(ctx, name, idx, total, nil)
			if err != nil {
				return err
			}

			groupConfig, err := md.appConfig.Flatten(name)
			if err != nil {
				return err
			}

			services := groupConfig.AllServices()
			for _, s := range services {
				if s.AutoStopMachines != nil && *s.AutoStopMachines == true {
					groupsWithAutostopEnabled[name] = true
				}
			}

			// Create spare machines that increases availability unless --ha=false was used
			if !md.increasedAvailability {
				continue
			}

			// We strive to provide a HA setup according to:
			// - Create only 1 machine if the group has mounts
			// - Create 2 machines for groups with services
			// - Create 1 always-on and 1 standby machine for groups without services
			switch {
			case len(groupConfig.Mounts) > 0:
				continue
			case len(services) > 0:
				fmt.Fprintf(md.io.Out, "Creating a second machine to increase service availability\n")
				if _, err := md.spawnMachineInGroup(ctx, name, idx, total, nil); err != nil {
					return err
				}
			default:
				fmt.Fprintf(md.io.Out, "Creating a standby machine for %s\n", md.colorize.Bold(machineID))
				standbyFor := []string{machineID}
				if _, err := md.spawnMachineInGroup(ctx, name, idx, total, standbyFor); err != nil {
					return err
				}
			}
		}
		fmt.Fprintf(md.io.ErrOut, "Finished launching new machines\n")

		if len(groupsWithAutostopEnabled) > 0 {
			groupNames := lo.Keys(groupsWithAutostopEnabled)
			slices.Sort(groupNames)
			fmt.Fprintf(md.io.Out,
				"\n%s The machines for [%s] have services with 'auto_stop_machines = true' that will be stopped when idling\n\n",
				md.colorize.Yellow("NOTE:"),
				md.colorize.Bold(strings.Join(groupNames, ",")),
			)
		}
	}

	machinesInPrimaryRegionPerGroup := make(map[string][]*api.MachineConfig)
	minRunningMachinesPerGroup := make(map[string]int)
	for _, lm := range md.machineSet.GetMachines() {
		machine := lm.Machine()
		processGroup := machine.Config.ProcessGroup()

		minRunningMachinesPerGroup[processGroup] = 1
		if mrm := machine.Config.Services[0].MinRunningMachines; mrm != nil {
			minRunningMachinesPerGroup[processGroup] = 1
		}

		if machine.Region == md.appConfig.PrimaryRegion {
			if machinesInGroup := machinesInPrimaryRegionPerGroup[processGroup]; machinesInGroup != nil {
				// Probably need a pointer here so we don't have to reassign
				machinesInGroup = append(machinesInGroup, machine.Config)
				machinesInPrimaryRegionPerGroup[processGroup] = machinesInGroup
			} else {
				machines := make([]*api.MachineConfig, 1)
				machines[0] = machine.Config
				machinesInPrimaryRegionPerGroup[processGroup] = machines
			}
		}
	}

	for processGroup, minRunningMachines := range minRunningMachinesPerGroup {
		if machinesInGroup := machinesInPrimaryRegionPerGroup[processGroup]; machinesInGroup != nil {
			if len(machinesInGroup) < minRunningMachines {
				fmt.Fprintf(md.io.Out,
					"\n%s There are %d machines in process group %s in the primary region but requested %d to keep running. All instances will keep running",
					md.colorize.Yellow("WARN:"),
					len(machinesInGroup),
					md.colorize.Bold(processGroup),
					minRunningMachines,
				)

				// Update minRunningMachines to be the length of machinesInGroup so we don't
				// cause a bounds error
				minRunningMachines = len(machinesInGroup)
			}

			fmt.Fprintf(md.io.Out,
				"%d machines will keep running in the primary region\n",
				minRunningMachines,
			)

			for i := 0; i < minRunningMachines; i++ {
				m := machinesInGroup[i]
				for i, svc := range m.Services {
					svc.Autostop = api.Pointer(false)
					m.Services[i] = svc
				}
			}
		}
	}

	var machineUpdateEntries []*machineUpdateEntry
	for _, lm := range md.machineSet.GetMachines() {
		li, err := md.launchInputForUpdate(lm.Machine())
		if err != nil {
			return fmt.Errorf("failed to update machine configuration for %s: %w", lm.FormattedMachineId(), err)
		}
		machineUpdateEntries = append(machineUpdateEntries, &machineUpdateEntry{leasableMachine: lm, launchInput: li})
	}

	return md.updateExistingMachines(ctx, machineUpdateEntries)
}

type machineUpdateEntry struct {
	leasableMachine machine.LeasableMachine
	launchInput     *api.LaunchMachineInput
}

func formatIndex(n, total int) string {
	pad := 0
	for i := total; i != 0; i /= 10 {
		pad++
	}
	return fmt.Sprintf("[%0*d/%d]", pad, n+1, total)
}

func (md *machineDeployment) updateExistingMachines(ctx context.Context, updateEntries []*machineUpdateEntry) error {
	// FIXME: handle deploy strategy: rolling, immediate, canary, bluegreen
	fmt.Fprintf(md.io.Out, "Updating existing machines in '%s' with %s strategy\n", md.colorize.Bold(md.app.Name), md.strategy)
	for i, e := range updateEntries {
		lm := e.leasableMachine
		launchInput := e.launchInput
		indexStr := formatIndex(i, len(updateEntries))

		if launchInput.ID != lm.Machine().ID {
			// If IDs don't match, destroy the original machine and launch a new one
			// This can be the case for machines that changes its volumes or any other immutable config
			fmt.Fprintf(md.io.ErrOut, "  %s Replacing %s by new machine\n", indexStr, md.colorize.Bold(lm.FormattedMachineId()))
			if err := lm.Destroy(ctx, true); err != nil {
				if md.strategy != "immediate" {
					return err
				}
				fmt.Fprintf(md.io.ErrOut, "Continuing after error: %s\n", err)
			}

			// Acquire a lease on the new machine to ensure external factors can't stop or update it
			// while we wait for its state and/or health checks
			launchInput.LeaseTTL = int(md.waitTimeout.Seconds())

			newMachineRaw, err := md.flapsClient.Launch(ctx, *launchInput)
			if err != nil {
				if md.strategy != "immediate" {
					return err
				}
				fmt.Fprintf(md.io.ErrOut, "Continuing after error: %s\n", err)
				continue
			}

			lm = machine.NewLeasableMachine(md.flapsClient, md.io, newMachineRaw)
			fmt.Fprintf(md.io.ErrOut, "  %s Created machine %s\n", indexStr, md.colorize.Bold(lm.FormattedMachineId()))
			defer lm.ReleaseLease(ctx)

		} else {
			fmt.Fprintf(md.io.ErrOut, "  %s Updating %s\n", indexStr, md.colorize.Bold(lm.FormattedMachineId()))
			if err := lm.Update(ctx, *launchInput); err != nil {
				if md.strategy != "immediate" {
					return err
				}
				fmt.Fprintf(md.io.ErrOut, "Continuing after error: %s\n", err)
			}
		}

		// Don't wait for Standby machines, they are updated but not started
		if len(launchInput.Config.Standbys) > 0 {
			md.logClearLinesAbove(1)
			fmt.Fprintf(md.io.ErrOut, "  %s Machine %s update finished: %s\n",
				indexStr,
				md.colorize.Bold(lm.FormattedMachineId()),
				md.colorize.Green("success"),
			)
			continue
		}

		if md.strategy == "immediate" {
			continue
		}

		if err := lm.WaitForState(ctx, api.MachineStateStarted, md.waitTimeout, indexStr); err != nil {
			return err
		}

		if err := md.doSmokeChecks(ctx, lm, indexStr); err != nil {
			return err
		}

		if !md.skipHealthChecks {
			if err := lm.WaitForHealthchecksToPass(ctx, md.waitTimeout, indexStr); err != nil {
				md.warnAboutIncorrectListenAddress(ctx, lm)
				return err
			}
			// FIXME: combine this wait with the wait for start as one update line (or two per in noninteractive case)
			md.logClearLinesAbove(1)
			fmt.Fprintf(md.io.ErrOut, "  %s Machine %s update finished: %s\n",
				indexStr,
				md.colorize.Bold(lm.FormattedMachineId()),
				md.colorize.Green("success"),
			)
		}

		md.warnAboutIncorrectListenAddress(ctx, lm)
	}

	fmt.Fprintf(md.io.ErrOut, "  Finished deploying\n")
	return nil
}

func (md *machineDeployment) spawnMachineInGroup(ctx context.Context, groupName string, i, total int, standbyFor []string) (string, error) {
	launchInput, err := md.launchInputForLaunch(groupName, md.machineGuest, standbyFor)
	if err != nil {
		return "", fmt.Errorf("error creating machine configuration: %w", err)
	}

	// Acquire a lease on the new machine to ensure external factors can't stop or update it
	// while we wait for its state and/or health checks
	launchInput.LeaseTTL = int(md.waitTimeout.Seconds())

	newMachineRaw, err := md.flapsClient.Launch(ctx, *launchInput)
	if err != nil {
		relCmdWarning := ""
		if strings.Contains(err.Error(), "please add a payment method") && !md.releaseCommandMachine.IsEmpty() {
			relCmdWarning = "\nPlease note that release commands run in their own ephemeral machine, and therefore count towards the machine limit."
		}
		return "", fmt.Errorf("error creating a new machine: %w%s", err, relCmdWarning)
	}

	lm := machine.NewLeasableMachine(md.flapsClient, md.io, newMachineRaw)
	fmt.Fprintf(md.io.ErrOut, "  Machine %s was created\n", md.colorize.Bold(lm.FormattedMachineId()))
	defer lm.ReleaseLease(ctx)

	// Don't wait for Standby machines, they are created but not started
	if len(launchInput.Config.Standbys) > 0 {
		return newMachineRaw.ID, nil
	}

	// Roll up as fast as possible when using immediate strategy
	if md.strategy == "immediate" {
		return newMachineRaw.ID, nil
	}

	// Otherwise wait for the machine to start
	indexStr := formatIndex(i, total)
	if err := lm.WaitForState(ctx, api.MachineStateStarted, md.waitTimeout, indexStr); err != nil {
		return "", err
	}

	if err := md.doSmokeChecks(ctx, lm, indexStr); err != nil {
		return "", err
	}

	// And wait (or not) for successful health checks
	if !md.skipHealthChecks {
		if err := lm.WaitForHealthchecksToPass(ctx, md.waitTimeout, indexStr); err != nil {
			md.warnAboutIncorrectListenAddress(ctx, lm)
			return "", err
		}

		md.logClearLinesAbove(1)
		fmt.Fprintf(md.io.ErrOut, "  Machine %s update finished: %s\n",
			md.colorize.Bold(lm.FormattedMachineId()),
			md.colorize.Green("success"),
		)
	}

	md.warnAboutIncorrectListenAddress(ctx, lm)

	return newMachineRaw.ID, nil
}

func (md *machineDeployment) resolveProcessGroupChanges() ProcessGroupsDiff {
	output := ProcessGroupsDiff{
		groupsToRemove:        map[string]int{},
		groupsNeedingMachines: map[string]bool{},
	}

	groupsInConfig := md.appConfig.ProcessNames()
	groupHasMachine := map[string]bool{}

	for _, leasableMachine := range md.machineSet.GetMachines() {
		name := leasableMachine.Machine().ProcessGroup()
		if slices.Contains(groupsInConfig, name) {
			groupHasMachine[name] = true
		} else {
			output.groupsToRemove[name] += 1
			output.machinesToRemove = append(output.machinesToRemove, leasableMachine)
		}
	}

	for _, name := range groupsInConfig {
		if ok := groupHasMachine[name]; !ok {
			output.groupsNeedingMachines[name] = true
		}
	}

	return output
}

func (md *machineDeployment) warnAboutProcessGroupChanges(ctx context.Context, diff ProcessGroupsDiff) {
	willAddMachines := len(diff.groupsNeedingMachines) != 0
	willRemoveMachines := diff.machinesToRemove != nil

	if !willAddMachines && !willRemoveMachines {
		return
	}

	fmt.Fprintln(md.io.Out, "Process groups have changed. This will:")

	if willRemoveMachines {
		bullet := md.colorize.Red("*")
		for grp, numMach := range diff.groupsToRemove {
			pluralS := lo.Ternary(numMach == 1, "", "s")
			fmt.Fprintf(md.io.Out, " %s destroy %d \"%s\" machine%s\n", bullet, numMach, grp, pluralS)
		}
	}
	if willAddMachines {
		bullet := md.colorize.Green("*")
		for name := range diff.groupsNeedingMachines {
			fmt.Fprintf(md.io.Out, " %s create 1 \"%s\" machine\n", bullet, name)
		}
	}
	fmt.Fprint(md.io.Out, "\n")
}

func (md *machineDeployment) warnAboutIncorrectListenAddress(ctx context.Context, lm machine.LeasableMachine) {
	group := lm.Machine().ProcessGroup()

	if _, ok := md.listenAddressChecked[group]; ok {
		return
	}
	md.listenAddressChecked[group] = struct{}{}

	groupConfig, err := md.appConfig.Flatten(group)
	if err != nil {
		return
	}
	services := groupConfig.AllServices()

	tcpServices := make(map[int]struct{})
	for _, s := range services {
		if s.Protocol == "tcp" {
			tcpServices[s.InternalPort] = struct{}{}
		}
	}

	processes, err := md.flapsClient.GetProcesses(ctx, lm.Machine().ID)
	// Let's not fail the whole deployment because of this, as listen address check is just a warning
	if err != nil {
		return
	}

	var foundSockets int
	for _, proc := range processes {
		for _, ls := range proc.ListenSockets {
			foundSockets += 1

			host, portStr, err := net.SplitHostPort(ls.Address)
			if err != nil {
				continue
			}
			port, err := strconv.Atoi(portStr)
			if err != nil {
				continue
			}

			ip := net.ParseIP(host)

			// We don't know VM's internal ipv4 which is also a valid address to bind to.
			// Let's assume that whoever binds to a non-loopback address knows what they are doing.
			// If we expose this address to flyctl later, we can revisit this logic.
			if !ip.IsLoopback() {
				delete(tcpServices, port)
			}
		}
	}

	// This can either mean that nothing is listening or that VM is running old init that doesn't expose
	// listen sockets. Until we have a way to update init on already created VMs let's ignore this
	// and pretend that this is old init.
	if foundSockets == 0 {
		return
	}

	// All services are covered
	if len(tcpServices) == 0 {
		return
	}

	fmt.Fprintf(md.io.ErrOut, "\n%s The app is listening on the incorrect address and will not be reachable by fly-proxy.\n", md.colorize.Yellow("WARNING"))
	fmt.Fprintf(md.io.ErrOut, "Make sure your app is listening on the following addresses:\n")
	for port := range tcpServices {
		fmt.Fprintf(md.io.ErrOut, "  - %s\n", md.colorize.Green("0.0.0.0:"+strconv.Itoa(port)))
	}
	fmt.Fprintf(md.io.ErrOut, "Found these processes inside the machine with open listening sockets:\n")

	table := helpers.MakeSimpleTable(md.io.ErrOut, []string{"Process", "Addresses"})
	for _, proc := range processes {
		var addresses []string
		for _, ls := range proc.ListenSockets {
			if ls.Proto == "tcp" {
				addresses = append(addresses, ls.Address)
			}
		}
		if len(addresses) > 0 {
			table.Append([]string{proc.Command, strings.Join(addresses, ", ")})
		}
	}
	table.Render()
	fmt.Fprintf(md.io.ErrOut, "\n")
}

func (md *machineDeployment) doSmokeChecks(ctx context.Context, lm machine.LeasableMachine, indexStr string) (err error) {
	if md.skipSmokeChecks {
		return nil
	}

	if err = lm.WaitForSmokeChecksToPass(ctx, indexStr); err == nil {
		md.logClearLinesAbove(1)
		return nil
	}

	fmt.Fprintf(md.io.ErrOut, "Smoke checks for %s failed: %v\n", md.colorize.Bold(lm.Machine().ID), err)
	fmt.Fprintf(md.io.ErrOut, "Check its logs: here's the last lines below, or run 'fly logs -i %s':\n", lm.Machine().ID)
	logs, _, logErr := md.apiClient.GetAppLogs(ctx, md.app.Name, "", md.appConfig.PrimaryRegion, lm.Machine().ID)
	if logErr != nil {
		return fmt.Errorf("error getting release_command logs: %w", logErr)
	}
	for _, l := range logs {
		// Ideally we should use InstanceID here, but it's not available in the logs.
		if l.Timestamp >= lm.Machine().UpdatedAt {
			fmt.Fprintf(md.io.ErrOut, "  %s\n", l.Message)
		}
	}

	return fmt.Errorf("smoke checks for %s failed: %v", lm.Machine().ID, err)
}
