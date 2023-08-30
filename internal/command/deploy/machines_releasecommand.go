package deploy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/machine"
)

func (md *machineDeployment) runReleaseCommand(ctx context.Context) error {
	if md.appConfig.Deploy == nil || md.appConfig.Deploy.ReleaseCommand == "" {
		return nil
	}

	fmt.Fprintf(md.io.ErrOut, "Running %s release_command: %s\n",
		md.colorize.Bold(md.app.Name),
		md.appConfig.Deploy.ReleaseCommand,
	)
	err := md.createOrUpdateReleaseCmdMachine(ctx)
	if err != nil {
		return fmt.Errorf("error running release_command machine: %w", err)
	}
	releaseCmdMachine := md.releaseCommandMachine.GetMachines()[0]
	// FIXME: consolidate this wait stuff with deploy waits? Especially once we improve the outpu
	err = md.waitForReleaseCommandToFinish(ctx, releaseCmdMachine)
	if err != nil {
		return err
	}
	lastExitEvent, err := releaseCmdMachine.WaitForEventTypeAfterType(ctx, "exit", "start", md.releaseCmdTimeout, true)
	if err != nil {
		return fmt.Errorf("error finding the release_command machine %s exit event: %w", releaseCmdMachine.Machine().ID, err)
	}
	exitCode, err := lastExitEvent.Request.GetExitCode()
	if err != nil {
		return fmt.Errorf("error get release_command machine %s exit code: %w", releaseCmdMachine.Machine().ID, err)
	}
	if exitCode != 0 {
		time.Sleep(2 * time.Second) // Wait 2 secs to be sure logs have reached OpenSearch
		fmt.Fprintf(md.io.ErrOut, "Error release_command failed running on machine %s with exit code %s.\n",
			md.colorize.Bold(releaseCmdMachine.Machine().ID), md.colorize.Red(strconv.Itoa(exitCode)))
		fmt.Fprintf(md.io.ErrOut, "Check its logs: here's the last 100 lines below, or run 'fly logs -i %s':\n",
			releaseCmdMachine.Machine().ID)
		releaseCmdLogs, _, err := md.apiClient.GetAppLogs(ctx, md.app.Name, "", md.appConfig.PrimaryRegion, releaseCmdMachine.Machine().ID)
		if api.IsNotAuthenticatedError(err) {
			fmt.Fprintf(md.io.ErrOut, "Warn: not authorized to retrieve app logs (this can happen when using deploy tokens), so we can't show you what failed. Use `fly logs -i %s` or open the monitoring dashboard to see them: https://fly.io/apps/%s/monitoring?region=&instance=%s\n", releaseCmdMachine.Machine().ID, md.appConfig.AppName, releaseCmdMachine.Machine().ID)
		} else {
			if err != nil {
				return fmt.Errorf("error getting release_command logs: %w", err)
			}
			for _, l := range releaseCmdLogs {
				fmt.Fprintf(md.io.ErrOut, "  %s\n", l.Message)
			}
		}
		return fmt.Errorf("error release_command machine %s exited with non-zero status of %d", releaseCmdMachine.Machine().ID, exitCode)
	}
	md.logClearLinesAbove(1)
	fmt.Fprintf(md.io.ErrOut, "  release_command %s completed successfully\n", md.colorize.Bold(releaseCmdMachine.Machine().ID))
	return nil
}

// dedicatedHostIdMismatch checks if the dedicatedHostID on a machine is the same as the one set in the fly.toml
// a mismatch will result in a delete+recreate op
func dedicatedHostIdMismatch(m *api.Machine, ac *appconfig.Config) bool {
	return strings.TrimSpace(ac.HostDedicationID) != "" && m.HostDedicationID != ac.HostDedicationID
}

func (md *machineDeployment) createOrUpdateReleaseCmdMachine(ctx context.Context) error {
	if md.releaseCommandMachine.IsEmpty() {
		return md.createReleaseCommandMachine(ctx)
	}

	releaseCmdMachine := md.releaseCommandMachine.GetMachines()[0]

	if dedicatedHostIdMismatch(releaseCmdMachine.Machine(), md.appConfig) {
		if err := releaseCmdMachine.Destroy(ctx, true); err != nil {
			return fmt.Errorf("error destroying release_command machine: %w", err)
		}

		return md.createReleaseCommandMachine(ctx)
	}

	return md.updateReleaseCommandMachine(ctx)
}

func (md *machineDeployment) createReleaseCommandMachine(ctx context.Context) error {
	launchInput := md.launchInputForReleaseCommand(nil)
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

	if err := releaseCmdMachine.WaitForState(ctx, api.MachineStateStopped, md.waitTimeout, "", false); err != nil {
		err = suggestChangeWaitTimeout(err, "wait-timeout")
		return err
	}

	if err := md.releaseCommandMachine.AcquireLeases(ctx, md.leaseTimeout); err != nil {
		return err
	}
	defer md.releaseCommandMachine.ReleaseLeases(ctx) // skipcq: GO-S2307
	md.releaseCommandMachine.StartBackgroundLeaseRefresh(ctx, md.leaseTimeout, md.leaseDelayBetween)

	launchInput := md.launchInputForReleaseCommand(releaseCmdMachine.Machine())
	if err := releaseCmdMachine.Update(ctx, *launchInput); err != nil {
		return fmt.Errorf("error updating release_command machine: %w", err)
	}

	return nil
}

func (md *machineDeployment) launchInputForReleaseCommand(origMachineRaw *api.Machine) *api.LaunchMachineInput {
	if origMachineRaw == nil {
		origMachineRaw = &api.Machine{
			Region: md.appConfig.PrimaryRegion,
		}
	}
	// We can ignore the error because ToReleaseMachineConfig fails only
	// if it can't split the command and we test that at initialization
	mConfig, _ := md.appConfig.ToReleaseMachineConfig()
	mConfig.Guest = md.inferReleaseCommandGuest()
	mConfig.Image = md.img
	md.setMachineReleaseData(mConfig)

	return &api.LaunchMachineInput{
		Config:           mConfig,
		Region:           origMachineRaw.Region,
		HostDedicationID: md.appConfig.HostDedicationID,
	}
}

func (md *machineDeployment) inferReleaseCommandGuest() *api.MachineGuest {
	defaultGuest := api.MachinePresets[DefaultVMSize]
	desiredGuest := api.MachinePresets["shared-cpu-2x"]
	if mg := md.machineGuest; mg != nil && (mg.CPUKind != defaultGuest.CPUKind || mg.CPUs != defaultGuest.CPUs || mg.MemoryMB != defaultGuest.MemoryMB) {
		desiredGuest = mg
	}
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
	return helpers.Clone(desiredGuest)
}

func (md *machineDeployment) waitForReleaseCommandToFinish(ctx context.Context, releaseCmdMachine machine.LeasableMachine) error {
	err := releaseCmdMachine.WaitForState(ctx, api.MachineStateStarted, md.waitTimeout, "", false)
	if err != nil {
		var flapsErr *flaps.FlapsError
		if errors.As(err, &flapsErr) && flapsErr.ResponseStatusCode == http.StatusNotFound {
			// The machine exited and was destroyed quickly.
			return nil
		}
		err = suggestChangeWaitTimeout(err, "wait-timeout")
		return fmt.Errorf("error waiting for release_command machine %s to start: %w", releaseCmdMachine.Machine().ID, err)
	}
	err = releaseCmdMachine.WaitForState(ctx, api.MachineStateDestroyed, md.releaseCmdTimeout, "", true)
	if err != nil {
		err = suggestChangeWaitTimeout(err, "release-command-timeout")
		return fmt.Errorf("error waiting for release_command machine %s to finish running: %w", releaseCmdMachine.Machine().ID, err)
	}
	return nil
}
