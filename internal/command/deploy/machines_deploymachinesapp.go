package deploy

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	machcmd "github.com/superfly/flyctl/internal/command/machine"
	"github.com/superfly/flyctl/internal/machine"
	"golang.org/x/exp/slices"
)

type ProcessGroupsDiff struct {
	machinesToRemove      []machine.LeasableMachine
	groupsToRemove        map[string]int
	groupsNeedingMachines map[string]bool
}

func (md *machineDeployment) DeployMachinesApp(ctx context.Context) error {
	ctx = flaps.NewContext(ctx, md.flapsClient)
	if md.restartOnly {
		return md.restartMachinesApp(ctx)
	}
	return md.deployMachinesApp(ctx)
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
//   * Run release command
//   * Remove spare machines from removed groups
//   * Launch new machines on new groups
//   * Update existing machines
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
	if len(processGroupMachineDiff.groupsNeedingMachines) > 0 {
		for name := range processGroupMachineDiff.groupsNeedingMachines {
			if err := md.spawnMachineInGroup(ctx, name); err != nil {
				return err
			}
		}
		fmt.Fprintf(md.io.ErrOut, "Finished launching new machines\n")
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

func (md *machineDeployment) updateExistingMachines(ctx context.Context, updateEntries []*machineUpdateEntry) error {
	// FIXME: handle deploy strategy: rolling, immediate, canary, bluegreen
	fmt.Fprintf(md.io.Out, "Updating existing machines in '%s' with %s strategy\n", md.colorize.Bold(md.app.Name), md.strategy)
	for _, e := range updateEntries {
		lm := e.leasableMachine
		launchInput := e.launchInput

		if launchInput.ID != lm.Machine().ID {
			// If IDs don't match, destroy the original machine and launch a new one
			// This can be the case for machines that changes its volumes or any other immutable config
			fmt.Fprintf(md.io.ErrOut, "  Replacing %s by new machine\n", md.colorize.Bold(lm.FormattedMachineId()))
			if err := lm.Destroy(ctx, true); err != nil {
				if md.strategy != "immediate" {
					return err
				}
				fmt.Fprintf(md.io.ErrOut, "Continuing after error: %s\n", err)
			}

			newMachineRaw, err := md.flapsClient.Launch(ctx, *launchInput)
			if err != nil {
				if md.strategy != "immediate" {
					return err
				}
				fmt.Fprintf(md.io.ErrOut, "Continuing after error: %s\n", err)
				continue
			}

			lm = machine.NewLeasableMachine(md.flapsClient, md.io, newMachineRaw)
			fmt.Fprintf(md.io.ErrOut, "  Created machine %s\n", md.colorize.Bold(lm.FormattedMachineId()))

		} else {
			fmt.Fprintf(md.io.ErrOut, "  Updating %s\n", md.colorize.Bold(lm.FormattedMachineId()))
			if err := lm.Update(ctx, *launchInput); err != nil {
				if md.strategy != "immediate" {
					return err
				}
				fmt.Fprintf(md.io.ErrOut, "Continuing after error: %s\n", err)
			}
		}

		if md.strategy == "immediate" {
			continue
		}

		if err := lm.WaitForState(ctx, api.MachineStateStarted, md.waitTimeout); err != nil {
			return err
		}

		if !md.skipHealthChecks {
			if err := lm.WaitForHealthchecksToPass(ctx, md.waitTimeout); err != nil {
				return err
			}
			// FIXME: combine this wait with the wait for start as one update line (or two per in noninteractive case)
			md.logClearLinesAbove(1)
			fmt.Fprintf(md.io.ErrOut, "  Machine %s update finished: %s\n",
				md.colorize.Bold(lm.FormattedMachineId()),
				md.colorize.Green("success"),
			)
		}
	}

	fmt.Fprintf(md.io.ErrOut, "  Finished deploying\n")
	return nil
}

func (md *machineDeployment) spawnMachineInGroup(ctx context.Context, groupName string) error {
	if groupName == "" {
		// If the group is unspecified, it should have been translated to "app" by this point
		panic("spawnMachineInGroup requires a non-empty group name. this is a bug!")
	}
	fmt.Fprintf(md.io.Out, "No machines in group '%s', launching one new machine\n", md.colorize.Bold(groupName))
	launchInput, err := md.launchInputForLaunch(groupName, nil)
	if err != nil {
		return fmt.Errorf("error creating machine configuration: %w", err)
	}

	newMachineRaw, err := md.flapsClient.Launch(ctx, *launchInput)
	if err != nil {
		return fmt.Errorf("error creating a new machine machine: %w", err)
	}

	newMachine := machine.NewLeasableMachine(md.flapsClient, md.io, newMachineRaw)

	// FIXME: dry this up with release commands and non-empty update
	fmt.Fprintf(md.io.ErrOut, "  Created release_command machine %s\n", md.colorize.Bold(newMachineRaw.ID))
	if md.strategy != "immediate" {
		err := newMachine.WaitForState(ctx, api.MachineStateStarted, md.waitTimeout)
		if err != nil {
			return err
		}
	}
	if md.strategy != "immediate" && !md.skipHealthChecks {
		err := newMachine.WaitForHealthchecksToPass(ctx, md.waitTimeout)
		// FIXME: combine this wait with the wait for start as one update line (or two per in noninteractive case)
		if err != nil {
			return err
		} else {
			md.logClearLinesAbove(1)
			fmt.Fprintf(md.io.ErrOut, "  Machine %s update finished: %s\n",
				md.colorize.Bold(newMachine.FormattedMachineId()),
				md.colorize.Green("success"),
			)
		}
	}
	return nil
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
