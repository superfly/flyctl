package deploy

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/ctrlc"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

var (
	ErrAborted             = errors.New("deployment aborted by user")
	ErrWaitTimeout         = errors.New("wait for goroutine timeout")
	ErrCreateGreenMachine  = errors.New("failed to create green machines")
	ErrWaitForStartedState = errors.New("could not get all green machines into started state")
	ErrWaitForHealthy      = errors.New("could not get all green machines to be healthy")
	ErrMarkReadyForTraffic = errors.New("failed to mark green machines as ready")
	ErrDestroyBlueMachines = errors.New("failed to destroy previous deployment")
	ErrValidationError     = errors.New("app not in valid state for bluegreen deployments")
)

type blueGreen struct {
	greenMachines   []machine.LeasableMachine
	blueMachines    []*machineUpdateEntry
	flaps           *flaps.Client
	io              *iostreams.IOStreams
	colorize        *iostreams.ColorScheme
	clearLinesAbove func(count int)
	timeout         time.Duration
	aborted         atomic.Bool
	ctrlcHook       ctrlc.Handle

	hangingBlueMachines []string
}

func BlueGreenStrategy(md *machineDeployment, blueMachines []*machineUpdateEntry) *blueGreen {
	bg := &blueGreen{
		greenMachines:       []machine.LeasableMachine{},
		blueMachines:        blueMachines,
		flaps:               md.flapsClient,
		timeout:             md.waitTimeout,
		io:                  md.io,
		colorize:            md.colorize,
		clearLinesAbove:     md.logClearLinesAbove,
		aborted:             atomic.Bool{},
		hangingBlueMachines: []string{},
	}

	// Hook into Ctrl+C so that we can rollback the deployment when it's aborted.
	ctrlc.ClearHandlers()
	bg.ctrlcHook = ctrlc.Hook(func() {
		bg.aborted.Store(true)
	})

	return bg

}

func (bg *blueGreen) CreateGreenMachines(ctx context.Context) error {
	var greenMachines []machine.LeasableMachine

	for _, mach := range bg.blueMachines {
		launchInput := mach.launchInput
		launchInput.SkipServiceRegistration = true

		newMachineRaw, err := bg.flaps.Launch(ctx, *launchInput)
		if err != nil {
			return err
		}

		greenMachine := machine.NewLeasableMachine(bg.flaps, bg.io, newMachineRaw)
		defer greenMachine.ReleaseLease(ctx)

		greenMachines = append(greenMachines, greenMachine)

		fmt.Fprintf(bg.io.ErrOut, "  Created machine %s\n", bg.colorize.Bold(greenMachine.FormattedMachineId()))
	}

	bg.greenMachines = greenMachines
	return nil
}

type machineState struct {
	machineId string
	status    string
	complete  bool
}

func (bg *blueGreen) renderMachineStates(state map[string]machineState, lastChangedMachine *string) {
	firstRun := lastChangedMachine == nil

	renderRow := func(id, status string) string {
		return fmt.Sprintf("  Machine %s - %s", bg.colorize.Bold(id), bg.colorize.Green(status))
	}

	var rows []string
	// In interactive mode, and first run, print all machines, clearing previous output
	if bg.io.IsInteractive() || firstRun {
		for id, status := range state {
			rows = append(rows, renderRow(id, status.status))
		}
		sort.Strings(rows)
		if !firstRun {
			// no-op in non-interactive mode
			bg.clearLinesAbove(len(rows))
		}
	} else {
		// in non-interactive mode, just print the status of the machine that actually changed
		rows = append(rows, renderRow(*lastChangedMachine, state[*lastChangedMachine].status))
	}

	fmt.Fprintf(bg.io.ErrOut, "%s\n", strings.Join(rows, "\n"))
}

func (bg *blueGreen) WaitForMachines(
	machineIDToStatus map[string]machineState,
	getStatus func(machine.LeasableMachine, chan error, chan machineState)) error {

	wait := time.NewTicker(bg.timeout)
	errChan := make(chan error)
	statusChan := make(chan machineState, len(machineIDToStatus))

	// render initial state
	bg.renderMachineStates(machineIDToStatus, nil)

	for _, gm := range bg.greenMachines {
		if _, ok := machineIDToStatus[gm.FormattedMachineId()]; ok {
			go getStatus(gm, errChan, statusChan)
		}
	}

	for {

		if bg.aborted.Load() {
			return ErrAborted
		}

		select {
		case err := <-errChan:
			return err
		case <-wait.C:
			return ErrWaitTimeout
		case status := <-statusChan:
			previousStatus := machineIDToStatus[status.machineId]
			// only render if a status has changed
			if status.status != previousStatus.status {
				machineIDToStatus[status.machineId] = status
				bg.renderMachineStates(machineIDToStatus, &status.machineId)
			}
		}

		completed := 0
		for _, v := range machineIDToStatus {
			if v.complete {
				completed += 1
			}
		}
		if len(machineIDToStatus) == completed {
			return nil
		}
	}
}

func (bg *blueGreen) WaitForGreenMachinesToBeStarted(ctx context.Context) error {
	machineIDToState := map[string]machineState{}
	for _, gm := range bg.greenMachines {
		machineIDToState[gm.FormattedMachineId()] = machineState{
			gm.FormattedMachineId(),
			"created",
			false,
		}
	}
	getStatus := func(m machine.LeasableMachine, errChan chan error, statusChan chan machineState) {
		err := machine.WaitForStartOrStop(ctx, m.Machine(), "start", bg.timeout)
		if err != nil {
			errChan <- err
			return
		}
		statusChan <- machineState{m.FormattedMachineId(), "started", true}
	}
	return bg.WaitForMachines(machineIDToState, getStatus)
}

func (bg *blueGreen) WaitForGreenMachinesToBeHealthy(ctx context.Context) error {
	machineIDToHealthStatus := map[string]machineState{}
	for _, gm := range bg.greenMachines {

		// in some cases, not all processes have healthchecks setup
		// eg. processes that run background workers, etc.
		// there's no point checking for health, a started state is enough
		if len(gm.Machine().Checks) == 0 {
			continue
		}

		machineIDToHealthStatus[gm.FormattedMachineId()] = machineState{gm.FormattedMachineId(), "unchecked", false}
	}

	getStatus := func(m machine.LeasableMachine, errChan chan error, statusChan chan machineState) {
		waitCtx, cancel := context.WithTimeout(ctx, bg.timeout)
		defer cancel()

		interval, gracePeriod := m.GetMinIntervalAndMinGracePeriod()

		time.Sleep(gracePeriod)

		for {
			updateMachine, err := bg.flaps.Get(waitCtx, m.Machine().ID)

			switch {
			case waitCtx.Err() != nil:
				errChan <- waitCtx.Err()
				return
			case err != nil:
				errChan <- err
				return
			}

			status := updateMachine.TopLevelChecks()
			allHealthy := (status.Total == status.Passing) && (status.Total != 0)
			statusChan <- machineState{m.FormattedMachineId(), fmt.Sprintf("%d/%d passing", status.Passing, status.Total), allHealthy}

			if allHealthy {
				return
			}

			time.Sleep(interval)
		}
	}

	return bg.WaitForMachines(machineIDToHealthStatus, getStatus)
}

func (bg *blueGreen) MarkGreenMachinesAsReadyForTraffic(ctx context.Context) error {
	for _, gm := range bg.greenMachines {
		if bg.aborted.Load() {
			return ErrAborted
		}
		err := bg.flaps.UnCordon(ctx, gm.Machine().ID)
		if err != nil {
			return err
		}

		fmt.Fprintf(bg.io.ErrOut, "  Machine %s now ready\n", gm.FormattedMachineId())
	}

	return nil
}

func (bg *blueGreen) DestroyBlueMachines(ctx context.Context) error {
	for _, gm := range bg.blueMachines {
		if bg.aborted.Load() {
			return ErrAborted
		}
		err := gm.leasableMachine.Destroy(ctx, true)
		if err != nil {
			bg.hangingBlueMachines = append(bg.hangingBlueMachines, gm.launchInput.ID)
			continue
		}

		fmt.Fprintf(bg.io.ErrOut, "  Machine %s destroyed\n", bg.colorize.Bold(gm.leasableMachine.FormattedMachineId()))
	}
	return nil
}

func (bg *blueGreen) attachCustomTopLevelChecks() {
	for _, entry := range bg.blueMachines {
		for _, service := range entry.launchInput.Config.Services {
			servicePort := service.InternalPort
			serviceProtocol := service.Protocol

			for _, check := range service.Checks {
				cc := api.MachineCheck{
					Port:              check.Port,
					Type:              check.Type,
					Interval:          check.Interval,
					Timeout:           check.Timeout,
					GracePeriod:       check.GracePeriod,
					HTTPMethod:        check.HTTPMethod,
					HTTPPath:          check.HTTPPath,
					HTTPProtocol:      check.HTTPProtocol,
					HTTPSkipTLSVerify: check.HTTPSkipTLSVerify,
					HTTPHeaders:       check.HTTPHeaders,
				}

				if cc.Port == nil {
					cc.Port = &servicePort
				}

				if cc.Type == nil {
					cc.Type = &serviceProtocol
				}

				if entry.launchInput.Config.Checks == nil {
					entry.launchInput.Config.Checks = make(map[string]api.MachineCheck)
				}
				entry.launchInput.Config.Checks[fmt.Sprintf("bg_deployments_%s", *check.Type)] = cc
			}
		}
	}
}

func (bg *blueGreen) Deploy(ctx context.Context) error {

	defer bg.ctrlcHook.Done()

	if bg.aborted.Load() {
		return ErrAborted
	}

	bg.attachCustomTopLevelChecks()

	totalChecks := 0
	for _, entry := range bg.blueMachines {
		if len(entry.launchInput.Config.Checks) == 0 {
			fmt.Fprintf(bg.io.ErrOut, "\n[WARN] Machine %s doesn't have healthchecks setup. We won't check its health.", entry.leasableMachine.FormattedMachineId())
			continue
		}

		totalChecks++
	}

	if totalChecks == 0 {
		fmt.Fprintf(bg.io.ErrOut, "\n\nYou need to define at least 1 check in order to use blue-green deployments. Refer to https://fly.io/docs/reference/configuration/#services-tcp_checks\n")
		return ErrValidationError
	}

	fmt.Fprintf(bg.io.ErrOut, "\nCreating green machines\n")
	if err := bg.CreateGreenMachines(ctx); err != nil {
		return errors.Wrap(err, ErrCreateGreenMachine.Error())
	}

	if bg.aborted.Load() {
		return ErrAborted
	}

	// because computers are too fast and everyone deserves a break sometimes
	time.Sleep(300 * time.Millisecond)

	fmt.Fprintf(bg.io.ErrOut, "\nWaiting for all green machines to start\n")
	if err := bg.WaitForGreenMachinesToBeStarted(ctx); err != nil {
		return errors.Wrap(err, ErrWaitForStartedState.Error())
	}

	if bg.aborted.Load() {
		return ErrAborted
	}

	fmt.Fprintf(bg.io.ErrOut, "\nWaiting for all green machines to be healthy\n")
	if err := bg.WaitForGreenMachinesToBeHealthy(ctx); err != nil {
		return errors.Wrap(err, ErrWaitForHealthy.Error())
	}

	if bg.aborted.Load() {
		return ErrAborted
	}

	fmt.Fprintf(bg.io.ErrOut, "\nMarking green machines as ready\n")
	if err := bg.MarkGreenMachinesAsReadyForTraffic(ctx); err != nil {
		return errors.Wrap(err, ErrMarkReadyForTraffic.Error())
	}

	if bg.aborted.Load() {
		return ErrAborted
	}

	fmt.Fprintf(bg.io.ErrOut, "\nDestroying all blue machines\n")
	if err := bg.DestroyBlueMachines(ctx); err != nil {
		return errors.Wrap(err, ErrDestroyBlueMachines.Error())
	}

	fmt.Fprintf(bg.io.ErrOut, "\nDeployment Complete\n")
	return nil
}

func (bg *blueGreen) Rollback(ctx context.Context, err error) error {

	if strings.Contains(err.Error(), ErrDestroyBlueMachines.Error()) {
		fmt.Fprintf(bg.io.ErrOut, "\nFailed to destroy blue machines (%s)\n", strings.Join(bg.hangingBlueMachines, ","))
		fmt.Fprintf(bg.io.ErrOut, "\nYou can destroy them using `fly machines destroy --force <id>`")
		return nil
	}

	for _, mach := range bg.greenMachines {
		err := mach.Destroy(ctx, true)
		if err != nil {
			return err
		}
	}

	return nil
}
