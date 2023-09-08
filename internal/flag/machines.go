package flag

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
)

// Returns a MachineGuest based on the flags provided overwriting a default VM
func GetMachineGuest(ctx context.Context, guest *api.MachineGuest) (*api.MachineGuest, error) {
	defaultVMSize := api.DefaultVMSize
	if IsSpecified(ctx, "vm-gpu-model") {
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

	if IsSpecified(ctx, "vm-cpukind") {
		guest.CPUKind = GetString(ctx, "vm-cpukind")
		if k := guest.CPUKind; k != "shared" && k != "performance" {
			return nil, fmt.Errorf("--vm-cpukind must be set to 'shared' or 'performance'")
		}
	}

	if IsSpecified(ctx, "vm-gpu-model") {
		m := GetString(ctx, "vm-gpu-model")
		if m != "a100-40gb-pci" && m != "a100-80gb-sxm" {
			return nil, fmt.Errorf("--vm-gpu-model must be set to 'a100-40gb-pci' or 'a100-80gb-sxm'")
		}
		guest.GPUModel = m
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
		Name:        "vm-cpukind",
		Description: "The kind of CPU to use ('shared' or 'performance')",
	},
	Int{
		Name:        "vm-memory",
		Description: "Memory (in megabytes) to attribute to the VM",
		Aliases:     []string{"memory"},
	},
	String{
		Name:        "vm-gpu-model",
		Description: "If set, the GPU model to attach ('a100-40gb-pci' or 'a100-80gb-sxm')",
	},
}
