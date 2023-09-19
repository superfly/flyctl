package flag

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/superfly/flyctl/api"
)

var validGPUKinds = []string{"a100-pcie-40gb", "a100-sxm4-80gb"}

// Returns a MachineGuest based on the flags provided overwriting a default VM
func GetMachineGuest(ctx context.Context, guest *api.MachineGuest) (*api.MachineGuest, error) {
	defaultVMSize := api.DefaultVMSize
	if IsSpecified(ctx, "vm-gpu-kind") {
		defaultVMSize = api.DefaultGPUVMSize
	}

	if guest == nil {
		guest = &api.MachineGuest{}
		guest.SetSize(defaultVMSize)
	}

	if IsSpecified(ctx, "vm-size") {
		if err := guest.SetSize(GetString(ctx, "vm-size")); err != nil {
			return nil, err
		}
	}

	if IsSpecified(ctx, "vm-cpus") {
		guest.CPUs = GetInt(ctx, "vm-cpus")
		if guest.CPUs == 0 {
			return nil, fmt.Errorf("--vm-cpus cannot be zero")
		}
	}

	if IsSpecified(ctx, "vm-memory") {
		guest.MemoryMB = GetInt(ctx, "vm-memory")
		if guest.MemoryMB == 0 {
			return nil, fmt.Errorf("--vm-memory cannot be zero")
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
		if !slices.Contains(validGPUKinds, m) {
			return nil, fmt.Errorf("--vm-gpu-kind must be set to one of: %v", strings.Join(validGPUKinds, ", "))
		}
		guest.GPUKind = m
	}

	return guest, nil
}

var VMSizeFlags = Set{
	String{
		Name:        "vm-size",
		Description: `The VM size to set machines to. See "fly platform vm-sizes" for valid values`,
		Aliases:     []string{"size"},
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
	Int{
		Name:        "vm-memory",
		Description: "Memory (in megabytes) to attribute to the VM",
		Aliases:     []string{"memory"},
	},
	String{
		Name:        "vm-gpu-kind",
		Description: fmt.Sprintf("If set, the GPU model to attach (%v)", strings.Join(validGPUKinds, ", ")),
		Aliases:     []string{"vm-gpukind"},
		Hidden:      true,
	},
}
