package flag

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/docker/go-units"
	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
)

var (
	validGPUKinds  = []string{"a100-pcie-40gb", "a100-sxm4-80gb", "l40s", "a10", "none"}
	gpuKindAliases = map[string]string{
		"a100-40gb": "a100-pcie-40gb",
		"a100-80gb": "a100-sxm4-80gb",
	}
)

// Returns a MachineGuest based on the flags provided overwriting a default VM
func GetMachineGuest(ctx context.Context, guest *fly.MachineGuest) (*fly.MachineGuest, error) {
	defaultVMSize := fly.DefaultVMSize
	if IsSpecified(ctx, "vm-gpu-kind") {
		defaultVMSize = fly.DefaultGPUVMSize
	}

	if guest == nil {
		guest = &fly.MachineGuest{}
		guest.SetSize(defaultVMSize)
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

	if IsSpecified(ctx, "vm-cpu-kind") {
		guest.CPUKind = GetString(ctx, "vm-cpu-kind")
		if k := guest.CPUKind; k != "shared" && k != "performance" {
			return nil, fmt.Errorf("--vm-cpu-kind must be set to 'shared' or 'performance'")
		}
	}

	if IsSpecified(ctx, "vm-gpu-kind") {
		m := GetString(ctx, "vm-gpu-kind")
		m = lo.ValueOr(gpuKindAliases, m, m)
		if !slices.Contains(validGPUKinds, m) {
			return nil, fmt.Errorf("--vm-gpu-kind must be set to one of: %v", strings.Join(validGPUKinds, ", "))
		}
		if m == "none" {
			guest.GPUs = 0
			guest.GPUKind = ""
		} else {
			guest.GPUKind = m
			if guest.GPUs == 0 {
				guest.GPUs = 1
			}
		}
	}

	if IsSpecified(ctx, "vm-gpus") {
		guest.GPUs = GetInt(ctx, "vm-gpus")
		switch {
		case guest.GPUKind != "" && guest.GPUs == 0:
			return nil, fmt.Errorf("--vm-gpus must be greater than zero, got: %d", guest.GPUs)
		case guest.GPUKind == "" && guest.GPUs > 0:
			return nil, fmt.Errorf("--vm-gpus requires a GPU Model to be set, pass --vm-gpu-kind=X where X is one of: %v", strings.Join(validGPUKinds, ", "))
		case guest.GPUs < 0:
			return nil, fmt.Errorf("--vm-gpus must be greater than or equal to zero, got: %d", guest.GPUs)
		}
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
		Description: "Number of CPUs",
		Aliases:     []string{"cpus"},
	},
	String{
		Name:        "vm-cpu-kind",
		Description: "The kind of CPU to use ('shared' or 'performance')",
		Aliases:     []string{"vm-cpukind"},
	},
	String{
		Name:        "vm-memory",
		Description: "Memory (in megabytes) to attribute to the VM",
		Aliases:     []string{"memory"},
	},
	Int{
		Name:        "vm-gpus",
		Description: "Number of GPUs. Must also choose the GPU model with --vm-gpu-kind flag",
	},
	String{
		Name:        "vm-gpu-kind",
		Description: fmt.Sprintf("If set, the GPU model to attach (%v)", strings.Join(validGPUKinds, ", ")),
		Aliases:     []string{"vm-gpukind"},
	},
	String{
		Name:        "host-dedication-id",
		Description: "The dedication id of the reserved hosts for your organization (if any)",
	},
}
