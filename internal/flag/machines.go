package flag

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
)

// Returns a MachineGuest based on the flags provided overwriting a default VM
func GetMachineGuest(ctx context.Context, guest *api.MachineGuest) (*api.MachineGuest, error) {
	if guest == nil {
		guest = &api.MachineGuest{}
		guest.SetSize(api.DefaultVMSize)
	}

	if IsSpecified(ctx, "vm-size") {
		if err := guest.SetSize(GetString(ctx, "vm-size")); err != nil {
			return nil, err
		}
	}
	if IsSpecified(ctx, "vm-cpus") {
		guest.CPUs = GetInt(ctx, "vm-cpus")
		if guest.CPUs == 0 {
			return nil, fmt.Errorf("cannot have zero cpus")
		}
	}

	if IsSpecified(ctx, "vm-memory") {
		guest.MemoryMB = GetInt(ctx, "vm-memory")
		if guest.MemoryMB == 0 {
			return nil, fmt.Errorf("memory cannot be zero")
		}
	}

	if IsSpecified(ctx, "vm-cpukind") {
		guest.CPUKind = GetString(ctx, "vm-cpukind")
		if k := guest.CPUKind; k != "shared" && k != "performance" {
			return nil, fmt.Errorf("cpukind must be set to 'shared' or 'performance'")
		}
	}
	if IsSpecified(ctx, "vm-gpus") {
		guest.GPUs = GetInt(ctx, "vm-gpus")
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
	Int{
		Name:        "vm-gpus",
		Description: "Number of GPUs",
	},
}
