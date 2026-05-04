package machine

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"
)

var cpusPerKind = map[string][]int{
	"shared":      {1, 2, 4, 6, 8},
	"performance": {1, 2, 4, 6, 8, 10, 12, 14, 16, 32, 64, 128},
}

func Update(ctx context.Context, appName string, m *fly.Machine, input *fly.LaunchMachineInput) error {
	var (
		flapsClient    = flapsutil.ClientFromContext(ctx)
		io             = iostreams.FromContext(ctx)
		colorize       = io.ColorScheme()
		updatedMachine *fly.Machine
		err            error
	)

	if input != nil && input.Config != nil && input.Config.Guest != nil {
		var invalidConfigErr InvalidConfigErr
		invalidConfigErr.guest = input.Config.Guest
		// Check that there's a valid number of CPUs
		validNumCpus, ok := cpusPerKind[input.Config.Guest.CPUKind]
		if !ok {
			invalidConfigErr.Reason = invalidCpuKind

			return invalidConfigErr
		} else if !slices.Contains(validNumCpus, input.Config.Guest.CPUs) {
			invalidConfigErr.Reason = invalidNumCPUs

			return invalidConfigErr
		}

		if input.Config.Guest.CPUKind == "shared" && input.Config.Guest.MemoryMB%256 != 0 {
			invalidConfigErr.Reason = invalidMemorySize

			return invalidConfigErr
		} else if input.Config.Guest.CPUKind == "performance" && input.Config.Guest.MemoryMB%1024 != 0 {
			invalidConfigErr.Reason = invalidMemorySize

			return invalidConfigErr
		}

		// Check memory sizes
		var min_memory_size int

		switch input.Config.Guest.CPUKind {
		case "shared":
			min_memory_size = fly.MIN_MEMORY_MB_PER_SHARED_CPU * input.Config.Guest.CPUs
		case "performance":
			min_memory_size = fly.MIN_MEMORY_MB_PER_CPU * input.Config.Guest.CPUs
		}

		if min_memory_size > input.Config.Guest.MemoryMB {
			invalidConfigErr.Reason = memoryTooLow

			return invalidConfigErr
		}

		var maxMemory int

		switch input.Config.Guest.CPUKind {
		case "shared":
			maxMemory = input.Config.Guest.CPUs * fly.MAX_MEMORY_MB_PER_SHARED_CPU
		case "performance":
			maxMemory = input.Config.Guest.CPUs * fly.MAX_MEMORY_MB_PER_CPU
		}

		if input.Config.Guest.MemoryMB > maxMemory {
			invalidConfigErr.Reason = memoryTooHigh

			return invalidConfigErr
		}

	}

	fmt.Fprintf(io.Out, "Updating machine %s\n", colorize.Bold(m.ID))

	input.ID = m.ID
	updatedMachine, err = tracing.WorkflowStep(ctx, "update_machine", func(ctx context.Context) (*fly.Machine, error) {
		return flapsClient.Update(ctx, appName, *input, m.LeaseNonce)
	})
	if err != nil {
		return fmt.Errorf("could not update machine %s: %w", m.ID, err)
	}

	waitTimeout := time.Second * 300
	if input.Timeout != 0 {
		waitTimeout = time.Duration(input.Timeout) * time.Second
	}

	settledMachine, err := tracing.WorkflowStep(ctx, "wait_for_update", func(ctx context.Context) (*fly.Machine, error) {
		return waitForUpdate(ctx, appName, m, updatedMachine, waitTimeout)
	})
	if err != nil {
		return err
	}

	if settledMachine.State == "started" && !input.SkipHealthChecks {
		if err := tracing.WorkflowPhase(ctx, "wait_for_health_checks", func(ctx context.Context) error {
			return watch.MachinesChecks(ctx, appName, []*fly.Machine{settledMachine})
		}); err != nil {
			return fmt.Errorf("failed to wait for health checks to pass: %w", err)
		}
	}

	fmt.Fprintf(io.Out, "Machine %s updated successfully!\n", colorize.Bold(m.ID))

	return nil
}

// waitForUpdate waits for the accepted update's version to settle and verifies
// that it is still the current version.
func waitForUpdate(ctx context.Context, appName string, previous, target *fly.Machine, timeout time.Duration) (*fly.Machine, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	_, waitErr := waitForStartOrStop(ctx, appName, target, "settled", timeout, "", target.Version)
	current, err := VerifyUpdateApplied(ctx, flapsutil.ClientFromContext(ctx), appName, previous, target, waitErr)
	if err != nil {
		return nil, fmt.Errorf("error while waiting for machine to update: %w", err)
	}

	return current, nil
}

// VerifyUpdateApplied checks the current version after a successful wait or
// when the pinned version disappeared or entered a terminal state. The caller
// supplies the same deadline used for the state wait.
func VerifyUpdateApplied(ctx context.Context, client flapsutil.FlapsClient, appName string, previous, target *fly.Machine, waitErr error) (*fly.Machine, error) {
	if waitErr != nil && !isTerminalVersionWaitError(waitErr) {
		return nil, waitErr
	}

	for {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) && waitErr == nil {
				return nil, fmt.Errorf("could not confirm promotion of machine %s to version %s: %w", target.ID, target.Version, err)
			}
			if waitErr != nil {
				return nil, waitErr
			}

			return nil, err
		}
		current, err := client.Get(ctx, appName, target.ID)
		if err != nil {
			if waitErr != nil {
				return nil, waitErr
			}
			if ctx.Err() != nil {
				continue
			}

			return nil, fmt.Errorf("failed to get machine %s after update: %w", target.ID, err)
		}
		switch current.Version {
		case target.Version:
			if current.State == "failed" {
				return nil, fmt.Errorf("machine %s update failed: machine entered %q state", target.ID, current.State)
			}
			if waitErr != nil {
				return nil, waitErr
			}

			return current, nil
		case previous.Version:
			// A candidate can reach stopped before flyd promotes it. Probe the
			// candidate's failed state because a settled wait also accepts failed.
			if waitErr != nil {
				if !strings.Contains(waitErr.Error(), "machine reached terminal state failed") {
					return nil, waitErr
				}
			} else {
				probeErr := client.Wait(ctx, appName, target.ID,
					flaps.WithWaitVersion(target.Version), flaps.WithWaitStates("failed"), flaps.WithWaitTimeout(time.Second))
				if isTerminalVersionWaitError(probeErr) {
					return nil, probeErr
				}
				if probeErr != nil {
					select {
					case <-ctx.Done():
					case <-time.After(200 * time.Millisecond):
					}
					continue
				}
			}

			return nil, fmt.Errorf("machine %s update failed: machine remains on previous version %s (state: %s)", target.ID, current.Version, current.State)
		default:
			return nil, fmt.Errorf("machine %s update was superseded: expected version %s, found %s (state: %s)", target.ID, target.Version, current.Version, current.State)
		}
	}
}

type invalidConfigReason string

const (
	invalidCpuKind    invalidConfigReason = "invalid CPU kind"
	invalidNumCPUs    invalidConfigReason = "invalid number of CPUs"
	invalidMemorySize invalidConfigReason = "invalid memory size"
	memoryTooLow      invalidConfigReason = "memory size for config is too low"
	memoryTooHigh     invalidConfigReason = "memory size for config is too high"
)

type InvalidConfigErr struct {
	Reason invalidConfigReason
	guest  *fly.MachineGuest
}

func (e InvalidConfigErr) Description() string {
	switch e.Reason {
	case invalidCpuKind:
		return fmt.Sprintf("The CPU kind given: %s, is not valid", e.guest.CPUKind)
	case invalidNumCPUs:
		return fmt.Sprintf("For the CPU kind %s, %d CPUs is not valid", e.guest.CPUKind, e.guest.CPUs)
	case invalidMemorySize:
		return fmt.Sprintf("%dMiB of memory is not valid", e.guest.MemoryMB)
	case memoryTooLow:
		return fmt.Sprintf("For %s VMs with %d CPUs, %dMiB of memory is too low", e.guest.CPUKind, e.guest.CPUs, e.guest.MemoryMB)
	case memoryTooHigh:
		return fmt.Sprintf("For %s VMs with %d CPUs, %dMiB of memory is too high", e.guest.CPUKind, e.guest.CPUs, e.guest.MemoryMB)
	}

	return string(e.Reason)
}

func (e InvalidConfigErr) Suggestion() string {
	switch e.Reason {
	case invalidCpuKind:
		return fmt.Sprintf("Valid values are %v", maps.Keys(cpusPerKind))
	case invalidNumCPUs:
		validNumCpus := cpusPerKind[e.guest.CPUKind]

		return fmt.Sprintf("Valid numbers are %v", validNumCpus)
	case invalidMemorySize:
		incrementSize := 1024
		switch e.guest.CPUKind {
		case "shared":
			incrementSize = 256
		case "performance":
			incrementSize = 1024
		}

		suggestedSize := e.guest.MemoryMB - (e.guest.MemoryMB % incrementSize)
		if suggestedSize == 0 {
			suggestedSize = incrementSize
		}

		return fmt.Sprintf("Memory size must be in a %d MiB increment (%dMiB would work)", incrementSize, suggestedSize)
	case memoryTooLow:
		var min_memory_size int

		switch e.guest.CPUKind {
		case "shared":
			min_memory_size = fly.MIN_MEMORY_MB_PER_SHARED_CPU * e.guest.CPUs
		case "performance":
			min_memory_size = fly.MIN_MEMORY_MB_PER_CPU * e.guest.CPUs
		}

		return fmt.Sprintf("Try setting memory to a value >= %dMiB for the config changes requested", min_memory_size)

	case memoryTooHigh:
		var max_memory_size int
		switch e.guest.CPUKind {
		case "shared":
			max_memory_size = fly.MAX_MEMORY_MB_PER_SHARED_CPU * e.guest.CPUs
		case "performance":
			max_memory_size = fly.MAX_MEMORY_MB_PER_CPU * e.guest.CPUs
		}

		return fmt.Sprintf("Try setting memory to a value <= %dMiB for the config changes requested", max_memory_size)
	}

	return ""
}

func (e InvalidConfigErr) DocURL() string {
	return "https://docs.fly.io/machines/guides-examples/machine-sizing"
}

func (e InvalidConfigErr) Error() string {
	return string(e.Reason)
}
