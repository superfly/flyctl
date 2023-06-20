package deploy

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/pkg/errors"

	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

var (
	ErrAborted             = errors.New("deployment aborted by user")
	ErrWaitTimeout         = errors.New("wait for goroutine timeout")
	ErrCreateGreenMachine  = errors.New("failed to create green machines")
	ErrWaitForStartedState = errors.New("could not get all green machines into started state")
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
}

func BlueGreenStrategy(md *machineDeployment, blueMachines []*machineUpdateEntry) *blueGreen {
	bg := &blueGreen{
		greenMachines:   []machine.LeasableMachine{},
		blueMachines:    blueMachines,
		flaps:           md.flapsClient,
		timeout:         md.waitTimeout,
		io:              md.io,
		colorize:        md.colorize,
		clearLinesAbove: md.logClearLinesAbove,
		aborted:         atomic.Bool{},
		healthLock:      sync.RWMutex{},
		stateLock:       sync.RWMutex{},
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

func (bg *blueGreen) renderMachineStates(state map[string]int) func() {
	firstRun := true

	return func() {
		rows := []string{}
		bg.stateLock.RLock()
		for id, value := range state {
			status := "created"
			if value == 1 {
				status = "started"
			}
			rows = append(rows, fmt.Sprintf("  Machine %s - %s", bg.colorize.Bold(id), bg.colorize.Green(status)))
		}
		bg.stateLock.RUnlock()

		if !firstRun {
			bg.clearLinesAbove(len(rows))
		}

		sort.Strings(rows)

		fmt.Fprintf(bg.io.ErrOut, "%s\n", strings.Join(rows, "\n"))
		firstRun = false
	}
}

func allMachinesStarted(stateMap map[string]int) bool {
	started := 0
	for _, v := range stateMap {
		started += v
	}

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

	fmt.Fprintf(bg.io.ErrOut, "\nDeployment Complete\n")
	return nil
}

func (bg *blueGreen) Rollback(ctx context.Context, err error) error {

	for _, mach := range bg.greenMachines {
		err := mach.Destroy(ctx, true)
		if err != nil {
			return err
		}
	}

	return nil
}
