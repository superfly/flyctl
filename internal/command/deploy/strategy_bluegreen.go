package deploy

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
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
	ErrAborted = errors.New("deployment aborted by user")
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

func (bg *blueGreen) Deploy(ctx context.Context) error {

	fmt.Fprintf(bg.io.ErrOut, "\nDeployment Complete\n")
	return nil
}

func (bg *blueGreen) Rollback(ctx context.Context, err error) error {

	return nil
}
