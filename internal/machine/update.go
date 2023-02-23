package machine

import (
	"context"
	"fmt"
	"time"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"
)

func Update(ctx context.Context, m *api.Machine, input *api.LaunchMachineInput) error {
	var (
		flapsClient    = flaps.FromContext(ctx)
		io             = iostreams.FromContext(ctx)
		colorize       = io.ColorScheme()
		updatedMachine *api.Machine
		err            error
	)

	if input != nil && input.Config != nil && input.Config.Guest != nil {
		// Check that there's a valid number of CPUs
		var validNumCpus []int

		if input.Config.Guest.CPUKind == "shared" {
			validNumCpus = append(validNumCpus, 1, 2, 4, 6, 8)

		} else if input.Config.Guest.CPUKind == "performance" {
			validNumCpus = append(validNumCpus, 1, 2, 4, 6, 8, 10, 12, 14, 16)

		}

		validCpuNum := false

		for _, num := range validNumCpus {
			if num == input.Config.Guest.CPUs {
				validCpuNum = true
				break

			}
		}

		if !validCpuNum {
			return fmt.Errorf("invalid config: invalid number of CPUs for %s guest. Valid numbers are %v\nView more information here: https://fly.io/docs/about/pricing/#machines", input.Config.Guest.CPUKind, validNumCpus)

		}

		if input.Config.Guest.CPUKind == "shared" && input.Config.Guest.MemoryMB%256 != 0 {
			return fmt.Errorf("invalid config: invalid memory size %d; must be in 256 MiB increment (%d would work)\nView more information here: https://fly.io/docs/about/pricing/#machines", input.Config.Guest.MemoryMB, input.Config.Guest.MemoryMB-(input.Config.Guest.MemoryMB%256))
		} else if input.Config.Guest.CPUKind == "performance" && input.Config.Guest.MemoryMB%1024 != 0 {
			return fmt.Errorf("invalid config: invalid memory size %d; must be in 1024 MiB increment (%d would work)\nView more information here: https://fly.io/docs/about/pricing/#machines", input.Config.Guest.MemoryMB, input.Config.Guest.MemoryMB-(input.Config.Guest.MemoryMB%1024))

		}

		// Check memory sizes
		var min_memory_size int

		if input.Config.Guest.CPUKind == "shared" {
			min_memory_size = api.MIN_MEMORY_MB_PER_SHARED_CPU * input.Config.Guest.CPUs
		} else if input.Config.Guest.CPUKind == "performance" {
			min_memory_size = api.MIN_MEMORY_MB_PER_CPU * input.Config.Guest.CPUs
		}

		if min_memory_size > input.Config.Guest.MemoryMB {
			return fmt.Errorf("invalid config: for machines with %d CPUs, the minimum amount of memory is %d MiB\nView more information here: https://fly.io/docs/about/pricing/#machines", input.Config.Guest.CPUs, min_memory_size)

		}

		var maxMemory int

		if input.Config.Guest.CPUKind == "shared" {
			maxMemory = input.Config.Guest.CPUs * api.MAX_MEMORY_MB_PER_SHARED_CPU
		} else if input.Config.Guest.CPUKind == "performance" {
			maxMemory = input.Config.Guest.CPUs * api.MAX_MEMORY_MB_PER_CPU
		}

		if input.Config.Guest.MemoryMB > maxMemory {
			return fmt.Errorf("invalid config: for machines with %d CPUs, the maximum amount of memory is %d MiB\nView more information here: https://fly.io/docs/about/pricing/#machines", input.Config.Guest.CPUs, maxMemory)

		}

	}

	fmt.Fprintf(io.Out, "Updating machine %s\n", colorize.Bold(m.ID))

	input.ID = m.ID
	updatedMachine, err = flapsClient.Update(ctx, *input, m.LeaseNonce)
	if err != nil {
		return fmt.Errorf("could not stop machine %s: %w", input.ID, err)
	}

	waitForAction := "start"
	if m.Config.Schedule != "" {
		waitForAction = "stop"
	}

	if err := WaitForStartOrStop(ctx, updatedMachine, waitForAction, time.Minute*5); err != nil {
		return err
	}

	if !input.SkipHealthChecks {
		if err := watch.MachinesChecks(ctx, []*api.Machine{updatedMachine}); err != nil {
			return fmt.Errorf("failed to wait for health checks to pass: %w", err)
		}
	}

	fmt.Fprintf(io.Out, "Machine %s updated successfully!\n", colorize.Bold(m.ID))

	return nil
}
