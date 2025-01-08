package machine

import (
	"context"
	"fmt"
	"slices"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"
	"golang.org/x/exp/maps"
)

var cpusPerKind = map[string][]int{
	"shared":      {1, 2, 4, 6, 8},
	"performance": {1, 2, 4, 6, 8, 10, 12, 14, 16, 32, 64, 128},
}

func Update(ctx context.Context, m *fly.Machine, input *fly.LaunchMachineInput) error {
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

		if input.Config.Guest.CPUKind == "shared" {
			min_memory_size = fly.MIN_MEMORY_MB_PER_SHARED_CPU * input.Config.Guest.CPUs
		} else if input.Config.Guest.CPUKind == "performance" {
			min_memory_size = fly.MIN_MEMORY_MB_PER_CPU * input.Config.Guest.CPUs
		}

		if min_memory_size > input.Config.Guest.MemoryMB {
			invalidConfigErr.Reason = memoryTooLow
			return invalidConfigErr
		}

		var maxMemory int

		if input.Config.Guest.CPUKind == "shared" {
			maxMemory = input.Config.Guest.CPUs * fly.MAX_MEMORY_MB_PER_SHARED_CPU
		} else if input.Config.Guest.CPUKind == "performance" {
			maxMemory = input.Config.Guest.CPUs * fly.MAX_MEMORY_MB_PER_CPU
		}

		if input.Config.Guest.MemoryMB > maxMemory {
			invalidConfigErr.Reason = memoryTooHigh
			return invalidConfigErr
		}

	}

	fmt.Fprintf(io.Out, "Updating machine %s\n", colorize.Bold(m.ID))

	input.ID = m.ID
	updatedMachine, err = flapsClient.Update(ctx, *input, m.LeaseNonce)
	if err != nil {
		return fmt.Errorf("could not update machine %s: %w", m.ID, err)
	}

	waitForAction := "start"
	if input.SkipLaunch || m.Config.Schedule != "" {
		waitForAction = "stop"
	}

	waitTimeout := time.Second * 300
	if input.Timeout != 0 {
		waitTimeout = time.Duration(input.Timeout) * time.Second
	}

	if err := WaitForStartOrStop(ctx, updatedMachine, waitForAction, waitTimeout); err != nil {
		return err
	}

	if !input.SkipLaunch {
		if !input.SkipHealthChecks {
			if err := watch.MachinesChecks(ctx, []*fly.Machine{updatedMachine}); err != nil {
				return fmt.Errorf("failed to wait for health checks to pass: %w", err)
			}
		}
	}

	fmt.Fprintf(io.Out, "Machine %s updated successfully!\n", colorize.Bold(m.ID))

	return nil
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
		var incrementSize int = 1024
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

		if e.guest.CPUKind == "shared" {
			min_memory_size = fly.MIN_MEMORY_MB_PER_SHARED_CPU * e.guest.CPUs
		} else if e.guest.CPUKind == "performance" {
			min_memory_size = fly.MIN_MEMORY_MB_PER_CPU * e.guest.CPUs
		}

		return fmt.Sprintf("Try setting memory to a value >= %dMiB for the config changes requested", min_memory_size)

	case memoryTooHigh:
		var max_memory_size int
		if e.guest.CPUKind == "shared" {
			max_memory_size = fly.MAX_MEMORY_MB_PER_SHARED_CPU * e.guest.CPUs
		} else if e.guest.CPUKind == "performance" {
			max_memory_size = fly.MAX_MEMORY_MB_PER_CPU * e.guest.CPUs
		}

		return fmt.Sprintf("Try setting memory to a value <= %dMiB for the config changes requested", max_memory_size)
	}

	return ""
}

func (e InvalidConfigErr) DocURL() string {
	return "https://fly.io/docs/machines/guides-examples/machine-sizing/"
}

func (e InvalidConfigErr) Error() string {
	return string(e.Reason)
}
