package flag

import (
	"context"
	"errors"
	"fmt"

	"github.com/docker/go-units"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
)

// Returns a MachineGuest based on the flags provided overwriting a default VM
func GetMachineGuest(ctx context.Context, guest *fly.MachineGuest) (*fly.MachineGuest, error) {
	if guest == nil {
		guest = &fly.MachineGuest{}
		guest.SetSize(fly.DefaultVMSize)
	}

	if IsSpecified(ctx, "vm-size") {
		if err := guest.SetSize(GetString(ctx, "vm-size")); err != nil {
			return nil, err
		}
	}

	if IsSpecified(ctx, "vm-cpus") {
		guest.CPUs = GetInt(ctx, "vm-cpus")
		if guest.CPUs <= 0 {
			return nil, fmt.Errorf("--vm-cpus must be greater than zero, got: %d", guest.CPUs)
		}
	}

	if IsSpecified(ctx, "vm-memory") {
		rawValue := GetString(ctx, "vm-memory")
		memoryMB, err := helpers.ParseSize(rawValue, units.RAMInBytes, units.MiB)
		switch {
		case err != nil:
			return nil, err
		case memoryMB == 0:
			return nil, fmt.Errorf("--vm-memory cannot be zero")
		default:
			guest.MemoryMB = memoryMB
		}
	}

	if IsSpecified(ctx, "vm-max-memory") {
		rawValue := GetString(ctx, "vm-max-memory")
		maxMemoryMB, err := helpers.ParseSize(rawValue, units.RAMInBytes, units.MiB)
		switch {
		case err != nil:
			return nil, err
		case maxMemoryMB == 0:
			return nil, fmt.Errorf("--vm-max-memory cannot be zero")
		default:
			guest.MaxMemoryMB = maxMemoryMB
		}
	}

	if IsSpecified(ctx, "vm-cpu-kind") {
		guest.CPUKind = GetString(ctx, "vm-cpu-kind")
		if k := guest.CPUKind; k != "shared" && k != "performance" {
			return nil, fmt.Errorf("--vm-cpu-kind must be set to 'shared' or 'performance'")
		}
	}

	if IsSpecified(ctx, "vm-gpu-kind") || IsSpecified(ctx, "vm-gpus") {
		return nil, errors.New("GPU machines are no longer supported: --vm-gpu-kind and --vm-gpus are no longer accepted")
	}

	if IsSpecified(ctx, "host-dedication-id") {
		guest.HostDedicationID = GetString(ctx, "host-dedication-id")
	}

	return guest, nil
}

var VMSizeFlags = Set{
	String{
		Name:        "vm-size",
		Description: `The VM size to set machines to. See "fly platform vm-sizes" for valid values`,
	},
	Int{
		Name:        "vm-cpus",
		Description: "Number of CPUs (also --cpus)",
		Aliases:     []string{"cpus"},
	},
	String{
		Name:        "vm-cpu-kind",
		Description: "The kind of CPU to use ('shared' or 'performance') (also --vm-cpukind)",
		Aliases:     []string{"vm-cpukind"},
	},
	String{
		Name:        "vm-memory",
		Description: "Memory (in megabytes) to attribute to the VM (also --memory)",
		Aliases:     []string{"memory"},
	},
	String{
		Name:        "vm-max-memory",
		Description: "Maximum memory (in megabytes) to allow for the VM",
		Hidden:      true,
	},
	// GPU machines are no longer supported. Both flags are kept so that
	// passing one fails with an explanation rather than "unknown flag".
	Int{
		Name:        "vm-gpus",
		Description: "GPU machines are no longer supported",
		Hidden:      true,
	},
	String{
		Name:        "vm-gpu-kind",
		Description: "GPU machines are no longer supported",
		Aliases:     []string{"vm-gpukind"},
		Hidden:      true,
	},
	String{
		Name:        "host-dedication-id",
		Description: "The dedication id of the reserved hosts for your organization (if any)",
	},
}
