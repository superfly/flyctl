package deploy

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
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
	healthLock      sync.RWMutex
	stateLock       sync.RWMutex
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
		healthLock:          sync.RWMutex{},
		stateLock:           sync.RWMutex{},
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

func (bg *blueGreen) renderMachineStates(state map[string]int) func() {
	firstRun := true

	lastReportedState := map[string]string{}

	return func() {
		rows := []string{}
		bg.stateLock.RLock()
		for id, value := range state {
			status := "created"
			if value == 1 {
				status = "started"
			}
			if !bg.io.IsInteractive() && lastReportedState[id] == status {
				continue
			}
			lastReportedState[id] = status
			rows = append(rows, fmt.Sprintf("  Machine %s - %s", bg.colorize.Bold(id), bg.colorize.Green(status)))
		}
		bg.stateLock.RUnlock()

		if !firstRun {
			bg.clearLinesAbove(len(rows))
		}

		sort.Strings(rows)

		if len(rows) == 0 {
			return
		}

		fmt.Fprintf(bg.io.ErrOut, "%s\n", strings.Join(rows, "\n"))
		firstRun = false
	}
}

func (bg *blueGreen) allMachinesStarted(stateMap map[string]int) bool {
	started := 0
	bg.stateLock.RLock()
	for _, v := range stateMap {
		started += v
	}
	bg.stateLock.RUnlock()

	return started == len(stateMap)
}

func (bg *blueGreen) WaitForGreenMachinesToBeStarted(ctx context.Context) error {
	wait := time.NewTicker(bg.timeout)
	machineIDToState := map[string]int{}
	render := bg.renderMachineStates(machineIDToState)
	errChan := make(chan error)

	for _, gm := range bg.greenMachines {
		machineIDToState[gm.FormattedMachineId()] = 0
	}

	for _, gm := range bg.greenMachines {
		id := gm.FormattedMachineId()

		if len(gm.Machine().Config.Standbys) > 0 {
			machineIDToState[id] = 1
			continue
		}

		go func(lm machine.LeasableMachine) {
			err := machine.WaitForStartOrStop(ctx, lm.Machine(), "start", bg.timeout)
			if err != nil {
				errChan <- err
				return
			}

			bg.stateLock.Lock()
			machineIDToState[id] = 1
			bg.stateLock.Unlock()
		}(gm)
	}

	for {
		if bg.allMachinesStarted(machineIDToState) {
			return nil
		}

		if bg.aborted.Load() {
			return ErrAborted
		}

		select {
		case <-wait.C:
			return ErrWaitTimeout
		case err := <-errChan:
			return err
		default:
			time.Sleep(90 * time.Millisecond)
			render()
		}
	}
}

func (bg *blueGreen) renderMachineHealthchecks(state map[string]*api.HealthCheckStatus) func() {
	firstRun := true

	lastReportedHealth := map[string]string{}

	return func() {
		rows := []string{}
		bg.healthLock.RLock()
		for id, value := range state {
			status := "unchecked"
			if value.Total != 0 {
				status = fmt.Sprintf("%d/%d passing", value.Passing, value.Total)
			}
			if !bg.io.IsInteractive() && lastReportedHealth[id] == status {
				continue
			}
			lastReportedHealth[id] = status
			rows = append(rows, fmt.Sprintf("  Machine %s - %s", bg.colorize.Bold(id), bg.colorize.Green(status)))
		}
		bg.healthLock.RUnlock()

		if !firstRun {
			bg.clearLinesAbove(len(rows))
		}

		sort.Strings(rows)

		if len(rows) == 0 {
			return
		}

		fmt.Fprintf(bg.io.ErrOut, "%s\n", strings.Join(rows, "\n"))
		firstRun = false
	}
}

func (bg *blueGreen) allMachinesHealthy(stateMap map[string]*api.HealthCheckStatus) bool {
	passed := 0

	bg.healthLock.RLock()
	for _, v := range stateMap {
		// we initialize all machine ids with an empty struct, so all fields are zero'd on init.
		// without v.hcs.Total != 0, the first call to this function will pass since 0 == 0
		if v.Total == 0 {
			continue
		}

		if v.Passing == v.Total {
			passed += 1
		}
	}
	bg.healthLock.RUnlock()

	return passed == len(stateMap)
}

func (bg *blueGreen) WaitForGreenMachinesToBeHealthy(ctx context.Context) error {
	wait := time.NewTicker(bg.timeout)
	machineIDToHealthStatus := map[string]*api.HealthCheckStatus{}
	errChan := make(chan error)
	render := bg.renderMachineHealthchecks(machineIDToHealthStatus)

	for _, gm := range bg.greenMachines {
		// in some cases, not all processes have healthchecks setup
		// eg. processes that run background workers, etc.
		// there's no point checking for health, a started state is enough
		if len(gm.Machine().Checks) == 0 {
			continue
		}

		machineIDToHealthStatus[gm.FormattedMachineId()] = &api.HealthCheckStatus{}
	}

	for _, gm := range bg.greenMachines {

		// in some cases, not all processes have healthchecks setup
		// eg. processes that run background workers, etc.
		// there's no point checking for health, a started state is enough
		if len(gm.Machine().Checks) == 0 {
			continue
		}

		go func(m machine.LeasableMachine) {
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
				bg.healthLock.Lock()
				machineIDToHealthStatus[m.FormattedMachineId()] = status
				bg.healthLock.Unlock()

				if (status.Total == status.Passing) && (status.Total != 0) {
					return
				}

				time.Sleep(interval)
			}

		}(gm)
	}

	for {

		if bg.allMachinesHealthy(machineIDToHealthStatus) {
			break
		}

		if bg.aborted.Load() {
			return ErrAborted
		}

		select {
		case err := <-errChan:
			return err
		case <-wait.C:
			return ErrWaitTimeout
		default:
			time.Sleep(90 * time.Millisecond)
			render()
		}
	}

	return nil
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
