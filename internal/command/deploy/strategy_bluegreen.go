package deploy

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/pkg/errors"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

var (
	ErrAborted             = errors.New("deployment aborted by user")
	ErrWaitTimeout         = errors.New("wait for goroutine timeout")
	ErrCreateGreenMachine  = errors.New("failed to create green machines")
	ErrWaitForStartedState = errors.New("could not get all green machines into started state")
	ErrWaitForHealthy      = errors.New("could not get all green machines to be healthy")
	ErrMarkReadyForTraffic = errors.New("failed to mark green machines as ready for traffic")
	ErrDestroyBlueMachines = errors.New("failed to destroy previous deployment")
)

type blueGreen struct {
	greenMachines []machine.LeasableMachine
	blueMachines  []*machineUpdateEntry

	flaps           *flaps.Client
	io              *iostreams.IOStreams
	colorize        *iostreams.ColorScheme
	clearLinesAbove func(count int)
	timeout         time.Duration
	aborted         atomic.Bool
}

func NewBlueGreenStrategy(md *machineDeployment, blueMachines []*machineUpdateEntry) *blueGreen {
	bg := &blueGreen{
		greenMachines:   []machine.LeasableMachine{},
		blueMachines:    blueMachines,
		flaps:           md.flapsClient,
		io:              md.io,
		colorize:        md.colorize,
		clearLinesAbove: md.logClearLinesAbove,
		aborted:         atomic.Bool{},

		// todo(@gwuah) - use the right value
		timeout: 2 * time.Minute,
	}

	// Hook into Ctrl+C so that aborting the migration
	// leaves the app in a stable, unlocked, non-detached state
	{
		signalCh := make(chan os.Signal, 1)
		abortSignals := []os.Signal{os.Interrupt}
		if runtime.GOOS != "windows" {
			abortSignals = append(abortSignals, syscall.SIGTERM)
		}
		// Prevent ctx from being cancelled, we need it to do recovery operations
		signal.Reset(abortSignals...)
		signal.Notify(signalCh, abortSignals...)
		go func() {
			<-signalCh
			// most terminals print ^C, this makes things easier to read.
			fmt.Fprintf(bg.io.ErrOut, "\n")
			bg.aborted.Store(true)
		}()
	}

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

		go func(lm machine.LeasableMachine) {
			err := machine.WaitForStartOrStop(ctx, lm.Machine(), "start", bg.timeout)
			if err != nil {
				errChan <- err
				return
			}

			machineIDToState[id] = 1
		}(gm)
	}

	for {
		if allMachinesStarted(machineIDToState) {
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

func (bg *blueGreen) WaitForGreenMachinesToBeHealthy(ctx context.Context) error {
	wait := time.NewTicker(bg.timeout)
	machineIDToHealthStatus := map[string]*api.HealthCheckStatus{}
	errChan := make(chan error)
	render := bg.renderMachineHealthchecks(machineIDToHealthStatus)

	for _, gm := range bg.greenMachines {
		machineIDToHealthStatus[gm.FormattedMachineId()] = &api.HealthCheckStatus{}
	}

	for _, gm := range bg.greenMachines {

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
				machineIDToHealthStatus[m.FormattedMachineId()] = status
				if status.Total == status.Passing {
					return
				}

				time.Sleep(interval)
			}

		}(gm)
	}

	for {

		if allMachinesHealthy(machineIDToHealthStatus) {
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
		err := bg.flaps.UnCordon(ctx, gm.Machine().ID)
		if err != nil {
			return err
		}

		fmt.Fprintf(bg.io.ErrOut, "  Machine %s now serving traffic\n", gm.FormattedMachineId())
	}

	return nil
}

func (bg *blueGreen) DestroyBlueMachines(ctx context.Context) error {
	for _, gm := range bg.blueMachines {
		err := gm.leasableMachine.Destroy(ctx, true)
		if err != nil {
			continue
		}

		fmt.Fprintf(bg.io.ErrOut, "  Machine %s destroyed\n", bg.colorize.Bold(gm.leasableMachine.FormattedMachineId()))
	}
	return nil
}

// helper methods sections

func allMachinesStarted(stateMap map[string]int) bool {
	started := 0
	for _, v := range stateMap {
		started += v
	}

	return started == len(stateMap)
}

func allMachinesHealthy(stateMap map[string]*api.HealthCheckStatus) bool {
	passed := 0

	for _, v := range stateMap {
		// we initialize all machine ids with an empty struct, so all fields are zero'd on init.
		// without v.hcs.Total != 0, the first call to this function will pass since 0 == 0
		if v.Passing == v.Total && v.Total != 0 {
			passed += 1
		}
	}

	return passed == len(stateMap)
}

func (bg *blueGreen) renderMachineStates(state map[string]int) func() {
	firstRun := true

	return func() {
		rows := []string{}
		for id, value := range state {
			status := "created"
			if value == 1 {
				status = "started"
			}
			rows = append(rows, fmt.Sprintf("  Machine %s - %s", bg.colorize.Bold(id), bg.colorize.Green(status)))
		}

		if !firstRun {
			bg.clearLinesAbove(len(rows))
		}

		sort.Strings(rows)

		fmt.Fprintf(bg.io.ErrOut, "%s\n", strings.Join(rows, "\n"))
		firstRun = false
	}
}

func (bg *blueGreen) renderMachineHealthchecks(state map[string]*api.HealthCheckStatus) func() {
	firstRun := true

	return func() {
		rows := []string{}
		for id, value := range state {
			status := "unchecked"
			if value.Total != 0 {
				status = fmt.Sprintf("%d/%d passing", value.Passing, value.Total)
			}
			rows = append(rows, fmt.Sprintf("  Machine %s - %s", bg.colorize.Bold(id), bg.colorize.Green(status)))
		}

		if !firstRun {
			bg.clearLinesAbove(len(rows))
		}

		sort.Strings(rows)

		fmt.Fprintf(bg.io.ErrOut, "%s\n", strings.Join(rows, "\n"))
		firstRun = false
	}
}

func (bg *blueGreen) Deploy(ctx context.Context) error {

	if bg.aborted.Load() {
		return ErrAborted
	}

	fmt.Fprintf(bg.io.Out, "\nCreating green machines\n")
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

	fmt.Fprintf(bg.io.ErrOut, "\nMarking green machines as ready for traffic\n")
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

// deployment cleanup section

func (bg *blueGreen) Rollback(ctx context.Context, err error) error {

	if strings.Contains(err.Error(), ErrDestroyBlueMachines.Error()) {
		// if we fail to destroy previoys deployment, there's no need to cancel the deployment
		// todo(@gwuah) - figure out what to do.
	}

	for _, mach := range bg.greenMachines {
		err := mach.Destroy(ctx, true)
		if err != nil {
			return err
		}
	}

	return nil
}
