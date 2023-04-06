package deploy

import (
	"context"
	"fmt"
	"strconv"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

func (md *machineDeployment) runReleaseCommand(ctx context.Context) error {
	if md.appConfig.Deploy == nil || md.appConfig.Deploy.ReleaseCommand == "" {
		return nil
	}

	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.ErrOut, "Running %s release_command: %s\n",
		md.colorize.Bold(md.app.Name),
		md.appConfig.Deploy.ReleaseCommand,
	)
	err := md.createOrUpdateReleaseCmdMachine(ctx)
	if err != nil {
		return fmt.Errorf("error running release_command machine: %w", err)
	}
	releaseCmdMachine := md.releaseCommandMachine.GetMachines()[0]
	// FIXME: consolidate this wait stuff with deploy waits? Especially once we improve the outpu
	err = releaseCmdMachine.WaitForState(ctx, api.MachineStateStarted, md.waitTimeout)
	if err != nil {
		return fmt.Errorf("error waiting for release_command machine %s to start: %w", releaseCmdMachine.Machine().ID, err)
	}
	err = releaseCmdMachine.WaitForState(ctx, api.MachineStateDestroyed, md.waitTimeout)
	if err != nil {
		return fmt.Errorf("error waiting for release_command machine %s to finish running: %w", releaseCmdMachine.Machine().ID, err)
	}
	lastExitEvent, err := releaseCmdMachine.WaitForEventTypeAfterType(ctx, "exit", "start", md.waitTimeout)
	if err != nil {
		return fmt.Errorf("error finding the release_command machine %s exit event: %w", releaseCmdMachine.Machine().ID, err)
	}
	exitCode, err := lastExitEvent.Request.GetExitCode()
	if err != nil {
		return fmt.Errorf("error get release_command machine %s exit code: %w", releaseCmdMachine.Machine().ID, err)
	}
	if exitCode != 0 {
		fmt.Fprintf(md.io.ErrOut, "Error release_command failed running on machine %s with exit code %s. Check the logs at: https://fly.io/apps/%s/monitoring\n",
			md.colorize.Bold(releaseCmdMachine.Machine().ID), md.colorize.Red(strconv.Itoa(exitCode)), md.app.Name)
		return fmt.Errorf("error release_command machine %s exited with non-zero status of %d", releaseCmdMachine.Machine().ID, exitCode)
	}
	md.logClearLinesAbove(1)
	fmt.Fprintf(md.io.ErrOut, "  release_command %s completed successfully\n", md.colorize.Bold(releaseCmdMachine.Machine().ID))
	return nil
}

func (md *machineDeployment) createOrUpdateReleaseCmdMachine(ctx context.Context) error {
	if md.releaseCommandMachine.IsEmpty() {
		return md.createReleaseCommandMachine(ctx)
	}
	return md.updateReleaseCommandMachine(ctx)
}

func (md *machineDeployment) createReleaseCommandMachine(ctx context.Context) error {
	launchInput := md.resolveUpdatedMachineConfig(nil, true)
	releaseCmdMachine, err := md.flapsClient.Launch(ctx, *launchInput)
	if err != nil {
		return fmt.Errorf("error creating a release_command machine: %w", err)
	}
	fmt.Fprintf(md.io.ErrOut, "  Created release_command machine %s\n", md.colorize.Bold(releaseCmdMachine.ID))
	md.releaseCommandMachine = machine.NewMachineSet(md.flapsClient, md.io, []*api.Machine{releaseCmdMachine})
	return nil
}

func (md *machineDeployment) updateReleaseCommandMachine(ctx context.Context) error {
	releaseCmdMachine := md.releaseCommandMachine.GetMachines()[0]
	fmt.Fprintf(md.io.ErrOut, "  Updating release_command machine %s\n", md.colorize.Bold(releaseCmdMachine.Machine().ID))
	err := releaseCmdMachine.WaitForState(ctx, api.MachineStateStopped, md.waitTimeout)
	if err != nil {
		return err
	}
	updatedConfig := md.resolveUpdatedMachineConfig(releaseCmdMachine.Machine(), true)
	err = md.releaseCommandMachine.AcquireLeases(ctx, md.leaseTimeout)
	defer func() {
		_ = md.releaseCommandMachine.ReleaseLeases(ctx)
	}()
	if err != nil {
		return err
	}
	err = releaseCmdMachine.Update(ctx, *updatedConfig)
	if err != nil {
		return fmt.Errorf("error updating release_command machine: %w", err)
	}
	return nil
}

func (md *machineDeployment) configureLaunchInputForReleaseCommand(launchInput *api.LaunchMachineInput) *api.LaunchMachineInput {
	desiredGuest := api.MachinePresets["shared-cpu-2x"]
	if !md.machineSet.IsEmpty() {
		group := md.appConfig.DefaultProcessName()
		ram := func(m *api.Machine) int {
			if m != nil && m.Config != nil && m.Config.Guest != nil {
				return m.Config.Guest.MemoryMB
			}
			return 0
		}

		maxRamMach := lo.Reduce(md.machineSet.GetMachines(), func(prevBest *api.Machine, lm machine.LeasableMachine, _ int) *api.Machine {
			mach := lm.Machine()
			if mach.ProcessGroup() != group {
				return prevBest
			}
			return lo.Ternary(ram(mach) > ram(prevBest), mach, prevBest)
		}, nil)
		if maxRamMach != nil {
			desiredGuest = maxRamMach.Config.Guest
		}
	}
	launchInput.Config.Guest = helpers.Clone(desiredGuest)
	return launchInput
}
