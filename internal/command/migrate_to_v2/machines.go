package migrate_to_v2

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

func (m *v2PlatformMigrator) createLaunchMachineInput(oldAllocID string, skipLaunch bool) (*api.LaunchMachineInput, error) {
	taskName := ""

	alloc, hasAlloc := lo.Find(m.oldAllocs, func(alloc *api.AllocationStatus) bool {
		return alloc.ID == oldAllocID
	})
	if hasAlloc {
		taskName = alloc.TaskName
	} else {
		taskName = "app"
	}

	mConfig, err := m.appConfig.ToMachineConfig(taskName, nil)
	if err != nil {
		return nil, err
	}

	guest, ok := m.machineGuests[mConfig.ProcessGroup()]
	if !ok {
		return nil, fmt.Errorf("no guest found for process '%s'", mConfig.ProcessGroup())
	}

	mConfig.Mounts = nil
	mConfig.Guest = guest
	mConfig.Image = m.img
	mConfig.Metadata[api.MachineConfigMetadataKeyFlyReleaseId] = m.releaseId
	mConfig.Metadata[api.MachineConfigMetadataKeyFlyReleaseVersion] = strconv.Itoa(m.releaseVersion)
	if oldAllocID != "" {
		mConfig.Metadata[api.MachineConfigMetadataKeyFlyPreviousAlloc] = oldAllocID
	}

	if m.isPostgres {
		mConfig.Env["FLY_CONSUL_URL"] = m.pgConsulUrl
		mConfig.Metadata[api.MachineConfigMetadataKeyFlyManagedPostgres] = "true"
	}

	if m.appConfig == nil {
		// FIXME better error message here
		return nil, fmt.Errorf("Could not find app config")
	}

	region := ""
	if hasAlloc {
		region = alloc.Region
	} else {
		region = m.appConfig.PrimaryRegion
	}

	// We have manual overrides for some regions with the names <region>2 e.g ams2, iad2.
	// These cause migrations to fail. Here we handle that specific case.
	if strings.HasSuffix(region, "2") {
		region = region[0:3]
	}

	launchMachineInput := api.LaunchMachineInput{
		Config:     mConfig,
		Region:     region,
		SkipLaunch: skipLaunch,
	}

	return &launchMachineInput, nil
}

func (m *v2PlatformMigrator) resolveMachineFromAlloc(alloc *api.AllocationStatus) (*api.LaunchMachineInput, error) {
	return m.createLaunchMachineInput(alloc.ID, false)
}

func (m *v2PlatformMigrator) prepMachinesToCreate(ctx context.Context) (err error) {
	m.newMachinesInput, err = m.resolveMachinesFromAllocs()
	if err != nil {
		return err
	}

	err = m.prepAutoscaleMachinesToCreate(ctx)
	return err
}

func (m *v2PlatformMigrator) prepAutoscaleMachinesToCreate(ctx context.Context) error {
	// If the service being migrated doesn't use autoscaling, just return nil
	if m.autoscaleConfig == nil || !m.autoscaleConfig.Enabled {
		return nil
	}

	// Create as many machines as necessary to be within the minimum count required
	for i := len(m.newMachinesInput); i < m.autoscaleConfig.MinCount; i += 1 {
		launchMachineInput, err := m.createLaunchMachineInput("", false)
		if err != nil {
			return fmt.Errorf("could not create machine to reach autoscale minimum count: %s", err)
		}

		m.newMachinesInput = append(m.newMachinesInput, launchMachineInput)
	}

	// Create the rest of the machines that app will use, but have them stopped by default
	for i := len(m.newMachinesInput); i < m.autoscaleConfig.MaxCount; i += 1 {
		launchMachineInput, err := m.createLaunchMachineInput("", true)
		if err != nil {
			return fmt.Errorf("could not create machine to reach autoscale minimum count: %s", err)
		}

		m.newMachinesInput = append(m.newMachinesInput, launchMachineInput)
		m.backupMachines[launchMachineInput.Config.ProcessGroup()] += 1
	}

	for _, input := range m.newMachinesInput {
		for i := range input.Config.Services {
			input.Config.Services[i].MinMachinesRunning = &m.autoscaleConfig.MinCount
			input.Config.Services[i].Autostart = api.BoolPointer(true)
			input.Config.Services[i].Autostop = api.BoolPointer(true)
		}
	}

	for i := range m.appConfig.Services {
		m.appConfig.Services[i].MinMachinesRunning = &m.autoscaleConfig.MinCount
		m.appConfig.Services[i].AutoStartMachines = api.BoolPointer(true)
		m.appConfig.Services[i].AutoStopMachines = api.BoolPointer(true)
	}

	return nil
}

func (m *v2PlatformMigrator) resolveMachinesFromAllocs() ([]*api.LaunchMachineInput, error) {
	var res []*api.LaunchMachineInput
	for _, alloc := range m.oldAllocs {
		mach, err := m.resolveMachineFromAlloc(alloc)
		if err != nil {
			return nil, err
		}
		res = append(res, mach)
	}
	return res, nil
}

type createdMachine struct {
	machine       *api.Machine
	expectedState string
}

func (m *v2PlatformMigrator) createMachines(ctx context.Context) error {
	var newlyCreatedMachines []createdMachine
	defer func() {
		m.recovery.machinesCreated = make([]*api.Machine, 0)

		for _, createdMachine := range newlyCreatedMachines {
			m.recovery.machinesCreated = append(m.recovery.machinesCreated, createdMachine.machine)
		}
	}()

	for _, machineInput := range m.newMachinesInput {
		if m.isPostgres && m.targetImg != "" {
			machineInput.Config.Image = m.targetImg
		}

		// Assign volume
		if nv, ok := lo.Find(m.createdVolumes, func(v *NewVolume) bool {
			return v.previousAllocId == machineInput.Config.Metadata[api.MachineConfigMetadataKeyFlyPreviousAlloc]
		}); ok {
			machineInput.Config.Mounts = []api.MachineMount{{
				Name:   nv.vol.Name,
				Path:   nv.mountPoint,
				Volume: nv.vol.ID,
			}}
		}

		// workaround for `maa` region deprecation
		if machineInput.Region == "maa" {
			io := iostreams.FromContext(ctx)
			fmt.Fprintf(io.Out, "Region 'maa' is deprecated, creating machine in fallback region 'bom'\n")
			machineInput.Region = "bom"
		}

		// Launch machine
		newMachine, err := m.flapsClient.Launch(ctx, *machineInput)
		if err != nil {
			return fmt.Errorf("failed creating a machine in region %s: %w", machineInput.Region, err)
		}

		expectedState := "start"
		if machineInput.SkipLaunch {
			expectedState = "stop"
		}

		machInfo := createdMachine{
			machine:       newMachine,
			expectedState: expectedState,
		}
		newlyCreatedMachines = append(newlyCreatedMachines, machInfo)

		if m.verbose {
			fmt.Fprintf(m.io.Out, "Created machine %s in %s\n", newMachine.ID, machineInput.Region)
		}
	}

	for _, mach := range newlyCreatedMachines {
		err := machine.WaitForStartOrStop(ctx, mach.machine, mach.expectedState, m.machineWaitTimeout)
		if err != nil {
			return err
		}
	}

	newlyCreatedMachinesSet := make([]*api.Machine, 0)

	for _, createdMachine := range newlyCreatedMachines {
		newlyCreatedMachinesSet = append(newlyCreatedMachinesSet, createdMachine.machine)
	}

	m.newMachines = machine.NewMachineSet(m.flapsClient, m.io, newlyCreatedMachinesSet)
	return nil
}
