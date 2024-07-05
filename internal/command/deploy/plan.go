package deploy

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flapsutil"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/statuslogger"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

type AppState struct {
	Machines []*fly.Machine
	Volumes  []fly.Volume
}

type machinePairing struct {
	oldMachine *fly.Machine
	newMachine *fly.Machine
}

func (md *machineDeployment) appState(ctx context.Context) (*AppState, error) {
	machines, err := md.flapsClient.List(ctx, "")
	if err != nil {
		return nil, err
	}
	volumes, err := md.flapsClient.GetVolumes(ctx)
	if err != nil {
		return nil, err
	}

	appState := &AppState{
		Machines: machines,
		Volumes:  volumes,
	}

	return appState, nil
}

func (md *machineDeployment) updateMachines(ctx context.Context, oldAppState, newAppState *AppState) error {
	// make a map of [machineID] -> [machine]
	oldMachines := make(map[string]*fly.Machine)
	for _, machine := range oldAppState.Machines {
		oldMachines[machine.ID] = machine
	}
	newMachines := make(map[string]*fly.Machine)
	for _, machine := range newAppState.Machines {
		newMachines[machine.ID] = machine
	}
	// First, we update the machines
	// Create a list of tuples of old and new machines
	machineTuples := make([]machinePairing, 0)

	// TODO: a little tired rn, do we need to do this?
	for _, oldMachine := range oldMachines {
		// This means we want to update a machine
		if newMachine, ok := newMachines[oldMachine.ID]; ok {
			machineTuples = append(machineTuples, machinePairing{oldMachine: oldMachine, newMachine: newMachine})
		} else {
			// This means we should destroy the old machine
			machineTuples = append(machineTuples, machinePairing{oldMachine: oldMachine, newMachine: nil})
			return fmt.Errorf("Machine not found in new state")
		}
	}

	for _, newMachine := range newMachines {
		if _, ok := oldMachines[newMachine.ID]; !ok {
			// This means we should create the new machine
			machineTuples = append(machineTuples, machinePairing{oldMachine: nil, newMachine: newMachine})
		}
	}

	sl := statuslogger.Create(ctx, len(machineTuples), true)
	defer sl.Destroy(false)

	group, ctx := errgroup.WithContext(ctx)
	for idx, machPair := range machineTuples {
		machPair := machPair
		oldMachine := machPair.oldMachine
		newMachine := machPair.newMachine

		idx := idx
		group.Go(func() error {
			err := updateMachine(ctx, oldMachine, newMachine, idx, sl)
			if err != nil {
				sl.Line(idx).LogStatus(statuslogger.StatusFailure, err.Error())
				return err
			}

			return nil

		})
	}

	if err := group.Wait(); err != nil {
		fmt.Println("Error updating machines", err)
		return err
	}

	return nil
}

func updateMachine(ctx context.Context, oldMachine, newMachine *fly.Machine, idx int, sl statuslogger.StatusLogger) error {
	var machine *fly.Machine
	var lease *fly.MachineLease

	// whether we need to create a new machine or update an existing one
	if oldMachine != nil {
		sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Acquiring lease for %s", oldMachine.ID))
		newLease, err := acquireMachineLease(ctx, oldMachine.ID)
		if err != nil {
			return err
		}
		lease = newLease

		sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Updating machine config for %s", oldMachine.ID))
		newMach, err := updateMachineConfig(ctx, oldMachine.ID, lease, newMachine.Config)
		if err != nil {
			return err
		}

		machine = newMach
	} else if newMachine != nil {
		sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Creating machine for %s", oldMachine.ID))
		machine, err := createMachine(ctx, newMachine.Config, newMachine.Region)
		if err != nil {
			return err
		}

		sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Acquiring lease for %s", oldMachine.ID))
		newLease, err := acquireMachineLease(ctx, machine.ID)
		if err != nil {
			return err
		}
		lease = newLease
	}
	// even if we fail to update the machine, we need to clear the lease
	defer func() {
		// clear the existing lease
		sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Clearing the lease for %s", oldMachine.ID))
		err := clearMachineLease(ctx, machine.ID, lease.Data.Nonce)
		if err != nil {
			sl.Line(idx).LogStatus(statuslogger.StatusFailure, fmt.Sprintf("Failed to clear lease for machine %s", oldMachine.ID))
		}
	}()

	var err error

	sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Waiting for machine %s to reach stopped/started state", oldMachine.ID))
	err = waitForMachineState(ctx, machine, []string{"stopped", "started", "suspended"}, 60*time.Second)
	if err != nil {
		return err
	}

	// start the machine
	sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Starting machine %s", oldMachine.ID))
	err = startMachine(ctx, machine.ID, lease.Data.Nonce)
	if err != nil {
		return err
	}

	// wait for the machine to reach the running state
	sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Waiting for machine %s to reach running state", oldMachine.ID))
	err = waitForMachineState(ctx, machine, []string{"started"}, 60*time.Second)
	if err != nil {
		return err
	}

	// check health of the machine
	sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Checking health of machine %s", oldMachine.ID))
	flapsClient := flapsutil.ClientFromContext(ctx)
	io := iostreams.FromContext(ctx)
	lm := mach.NewLeasableMachine(flapsClient, io, machine)

	err = lm.WaitForHealthchecksToPass(ctx, 60*time.Second)
	if err != nil {
		return err
	}

	return nil

}

func detectMultipleImageVersions(ctx context.Context) ([]*fly.Machine, error) {
	flapsClient := flapsutil.ClientFromContext(ctx)
	machines, err := flapsClient.List(ctx, "")
	if err != nil {
		return nil, err
	}

	// First, we get the latest image
	var latestImage string
	var latestUpdated time.Time

	for _, machine := range machines {
		updated, err := time.Parse(time.RFC3339, machine.UpdatedAt)
		if err != nil {
			return nil, err
		}

		if updated.After(latestUpdated) {
			latestUpdated = updated
			latestImage = machine.Config.Image
		}
	}

	var badMachines []*fly.Machine
	// Next, we find any machines that are not using the latest image
	for _, machine := range machines {
		if machine.Config.Image != latestImage {
			badMachines = append(badMachines, machine)
		}
	}

	return badMachines, nil
}

func clearMachineLease(ctx context.Context, machID, leaseNonce string) error {
	flapsClient := flapsutil.ClientFromContext(ctx)
	err := flapsClient.ReleaseLease(ctx, machID, leaseNonce)
	if err != nil {
		return err
	}

	return nil
}

func waitForMachineState(ctx context.Context, machine *fly.Machine, possibleStates []string, timeout time.Duration) error {
	var mutex sync.Mutex

	var waitErr error
	var completed bool

	for _, state := range possibleStates {
		state := state
		go func() {
			flapsClient := flapsutil.ClientFromContext(ctx)
			err := flapsClient.Wait(ctx, machine, state, timeout)
			mutex.Lock()
			defer mutex.Unlock()

			if err != nil && !completed {
				waitErr = err
			} else {
				completed = true
			}

		}()
	}

	for {
		mutex.Lock()
		if completed {
			defer mutex.Unlock()
			return waitErr
		}
		mutex.Unlock()
	}
}

func acquireMachineLease(ctx context.Context, machID string) (*fly.MachineLease, error) {
	flapsClient := flapsutil.ClientFromContext(ctx)
	lease, err := flapsClient.AcquireLease(ctx, machID, fly.IntPointer(3600))
	if err != nil {
		return nil, err
	}

	return lease, nil
}

func updateMachineConfig(ctx context.Context, machID string, lease *fly.MachineLease, machConfig *fly.MachineConfig) (*fly.Machine, error) {
	// First, let's get a lease on the machine
	flapsClient := flapsutil.ClientFromContext(ctx)
	mach, err := flapsClient.Update(ctx, fly.LaunchMachineInput{
		Config: machConfig,
		ID:     machID,
	}, lease.Data.Nonce)
	if err != nil {
		return nil, err
	}

	return mach, nil
}

func createMachine(ctx context.Context, machConfig *fly.MachineConfig, region string) (*fly.Machine, error) {
	flapsClient := flapsutil.ClientFromContext(ctx)
	machine, err := flapsClient.Launch(ctx, fly.LaunchMachineInput{
		Config: machConfig,
		Region: region,
	})
	if err != nil {
		return nil, err
	}

	return machine, nil
}

func (md *machineDeployment) updateMachine(ctx context.Context, e *machineUpdateEntry) error {
	ctx, span := tracing.GetTracer().Start(ctx, "update_machine", trace.WithAttributes(
		attribute.String("id", e.launchInput.ID),
		attribute.Bool("requires_replacement", e.launchInput.RequiresReplacement),
	))
	defer span.End()

	fmtID := e.leasableMachine.FormattedMachineId()

	replaceMachine := func() error {
		statuslogger.Logf(ctx, "Replacing %s by new machine", md.colorize.Bold(fmtID))
		if err := md.updateMachineByReplace(ctx, e); err != nil {
			return err
		}
		statuslogger.Logf(ctx, "Created machine %s", md.colorize.Bold(fmtID))
		return nil
	}

	if e.launchInput.RequiresReplacement {
		return replaceMachine()
	}

	statuslogger.Logf(ctx, "Updating %s", md.colorize.Bold(fmtID))
	if err := md.updateMachineInPlace(ctx, e); err != nil {
		switch {
		case len(e.leasableMachine.Machine().Config.Mounts) > 0:
			// Replacing a machine with a volume will cause the placement logic to pick wthe same host
			// dismissing the value of replacing it in case of lack of host capacity
			return err
		case strings.Contains(err.Error(), "could not reserve resource for machine"):
			return replaceMachine()
		default:
			return err
		}
	}
	return nil
}

func startMachine(ctx context.Context, machineID string, leaseNonce string) error {
	flapsClient := flapsutil.ClientFromContext(ctx)
	_, err := flapsClient.Start(ctx, machineID, leaseNonce)
	if err != nil {
		fmt.Println("Failed to start machine", machineID, "due to error", err)
		return err
	}

	return nil
}
