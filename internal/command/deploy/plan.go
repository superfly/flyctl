package deploy

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/ctrlc"
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

type updateMachinesErr struct {
	err                error
	successfulRollback bool
}

func (e *updateMachinesErr) Error() string {
	return fmt.Sprintf("failed to update machines: %s", e.err)
}

func (e *updateMachinesErr) Unwrap() error {
	return e.err
}

func (md *machineDeployment) updateMachines(ctx context.Context, oldAppState, newAppState *AppState, rollback bool, statusLogger statuslogger.StatusLogger) error {
	ctx, cancel := context.WithCancel(ctx)
	ctx, cancel = ctrlc.HookCancelableContext(ctx, cancel)
	defer cancel()
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
			// FIXME: this would currently delete unmanaged machines! no bueno
			// fmt.Println("Deleting machine", oldMachine.ID)
			// This means we should destroy the old machine
			// machineTuples = append(machineTuples, machinePairing{oldMachine: oldMachine, newMachine: nil})
		}
	}

	for _, newMachine := range newMachines {
		if _, ok := oldMachines[newMachine.ID]; !ok {
			// This means we should create the new machine
			machineTuples = append(machineTuples, machinePairing{oldMachine: nil, newMachine: newMachine})
		}
	}

	var sl statuslogger.StatusLogger
	if statusLogger != nil {
		sl = statusLogger
	} else {
		sl = statuslogger.Create(ctx, len(machineTuples), true)
		defer sl.Destroy(false)
	}

	group, ctx := errgroup.WithContext(ctx)
	for idx, machPair := range machineTuples {
		machPair := machPair
		oldMachine := machPair.oldMachine
		newMachine := machPair.newMachine

		idx := idx
		group.Go(func() error {
			err := updateMachine(ctx, oldMachine, newMachine, idx, sl, md.io)
			if err != nil {
				sl.Line(idx).LogStatus(statuslogger.StatusFailure, err.Error())
				return err
			}

			return nil
		})
	}

	if updateErr := group.Wait(); updateErr != nil {
		if !rollback {
			return updateErr
		}

		// no point in rolling back on a context canceled error
		if strings.Contains(updateErr.Error(), "context canceled") {
			return updateErr
		}

		// if we fail to update the machines, we should revert the state back if possible
		ctx = context.WithoutCancel(ctx)
		for {
			currentState, err := md.appState(ctx)
			if err != nil {
				fmt.Println("Failed to get current state:", err)
				return err
			}
			err = md.updateMachines(ctx, currentState, newAppState, false, sl)
			if err == nil {
				break
			} else if strings.Contains(err.Error(), "context canceled") {
				return err
			} else {
				fmt.Println("Failed to update machines:", err, ". Retrying...")
			}
			time.Sleep(1 * time.Second)
		}

		return nil
	}

	return nil
}

func updateMachine(ctx context.Context, oldMachine, newMachine *fly.Machine, idx int, sl statuslogger.StatusLogger, io *iostreams.IOStreams) error {
	if reflect.DeepEqual(oldMachine.Config, newMachine.Config) {
		// if the machine is already in the exact state we want it to be in, we  skip this
		sl.Line(idx).LogStatus(statuslogger.StatusSuccess, fmt.Sprintf("Machine %s is already in the desired state", oldMachine.ID))
		return nil
	}

	var machine *fly.Machine = oldMachine
	var lease *fly.MachineLease

	defer func() {
		if machine == nil || lease == nil {
			return
		}

		// even if we fail to update the machine, we need to clear the lease
		// clear the existing lease
		ctx := context.WithoutCancel(ctx)
		sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Clearing the lease for %s", machine.ID))
		err := clearMachineLease(ctx, machine.ID, lease.Data.Nonce)
		if err != nil {
			fmt.Println("Failed to clear lease for machine", machine.ID, "due to error", err)
			sl.Line(idx).LogStatus(statuslogger.StatusFailure, fmt.Sprintf("Failed to clear lease for machine %s", machine.ID))
		}
	}()

	// whether we need to create a new machine or update an existing one
	if oldMachine != nil {
		sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Acquiring lease for %s", oldMachine.ID))
		newLease, err := acquireMachineLease(ctx, oldMachine.ID)
		if err != nil {
			return err
		}
		lease = newLease

		if newMachine == nil {
			destroyMachine(ctx, oldMachine.ID, lease.Data.Nonce)
		} else {
			// if the config hasn't changed, we don't need to update the machine
			sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Updating machine config for %s", oldMachine.ID))
			newMachine, err := updateMachineConfig(ctx, oldMachine, newMachine.Config, lease)
			if err != nil {
				return err
			}
			machine = newMachine
		}
	} else if newMachine != nil {
		sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Creating machine for %s", newMachine.ID))
		var err error
		machine, err = createMachine(ctx, newMachine.Config, newMachine.Region)
		if err != nil {
			return err
		}

		sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Acquiring lease for %s", newMachine.ID))
		newLease, err := acquireMachineLease(ctx, machine.ID)
		if err != nil {
			return err
		}
		lease = newLease
	}

	var err error

	flapsClient := flapsutil.ClientFromContext(ctx)
	lm := mach.NewLeasableMachine(flapsClient, io, machine, false)

	sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Waiting for machine %s to reach a good state", oldMachine.ID))
	err = waitForMachineState(ctx, lm, []string{"stopped", "started", "suspended"}, 60*time.Second)
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
	err = waitForMachineState(ctx, lm, []string{"started"}, 60*time.Second)
	if err != nil {
		return err
	}

	// check health of the machine
	sl.Line(idx).LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Checking health of machine %s", oldMachine.ID))

	err = lm.WaitForHealthchecksToPass(ctx, 60*time.Second)
	if err != nil {
		return err
	}

	return nil
}

func destroyMachine(ctx context.Context, machineID string, lease string) error {
	flapsClient := flapsutil.ClientFromContext(ctx)
	err := flapsClient.Destroy(ctx, fly.RemoveMachineInput{
		ID:   machineID,
		Kill: true,
	}, lease)
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
	// TODO: remove this when valentin's work is done
	flapsClient := flapsutil.ClientFromContext(ctx)
	for {
		err := flapsClient.ReleaseLease(ctx, machID, leaseNonce)
		if err == nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
}

// returns when the machine is in one of the possible states, or after passing the timeout threshold
func waitForMachineState(ctx context.Context, lm mach.LeasableMachine, possibleStates []string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var mutex sync.Mutex

	var waitErr error
	numFinished := 0

	// something's wrong with waitForState, since sometimes we're already in the state we need but waitForState times out
	if lo.Contains(possibleStates, lm.Machine().State) {
		return nil
	}

	for _, state := range possibleStates {
		state := state
		go func() {
			err := lm.WaitForState(ctx, state, timeout, false)

			mutex.Lock()
			defer func() {
				numFinished += 1
				mutex.Unlock()
			}()

			if err != nil {
				waitErr = err
			}
		}()
	}

	// TODO(billy): i'm sure we can use channels here
	for {
		mutex.Lock()
		if numFinished == len(possibleStates) {
			mutex.Unlock()
			return waitErr
		}
		mutex.Unlock()

		time.Sleep(1 * time.Second)
	}
}

func acquireMachineLease(ctx context.Context, machID string) (*fly.MachineLease, error) {
	flapsClient := flapsutil.ClientFromContext(ctx)
	lease, err := flapsClient.AcquireLease(ctx, machID, fly.IntPointer(3600))
	if err != nil {
		// TODO: tell users how to manually clear the lease
		// TODO: have a flag to automatically clear the lease
		return nil, err
	}

	return lease, nil
}

func updateMachineConfig(ctx context.Context, oldMachine *fly.Machine, newMachineConfig *fly.MachineConfig, lease *fly.MachineLease) (*fly.Machine, error) {
	if reflect.DeepEqual(oldMachine.Config, newMachineConfig) {
		return oldMachine, nil
	}

	flapsClient := flapsutil.ClientFromContext(ctx)
	mach, err := flapsClient.Update(ctx, fly.LaunchMachineInput{
		Config: newMachineConfig,
		ID:     oldMachine.ID,
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
