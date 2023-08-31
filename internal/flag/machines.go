package flag

import (
	"context"

	"github.com/superfly/flyctl/api"
)

// Returns a MachineGuest based on the flags provided overwriting a default VM
func GetMachineGuest(ctx context.Context) *api.MachineGuest {
	var guest api.MachineGuest
	guest.SetSize(api.DefaultVMSize)

	if IsSpecified(ctx, "vm-size") {
		guest.SetSize(GetString(ctx, "vm-size"))
	}
	if IsSpecified(ctx, "vm-cpus") {
		guest.CPUs = GetInt(ctx, "vm-cpus")
	}
	if IsSpecified(ctx, "vm-memory") {
		guest.MemoryMB = GetInt(ctx, "vm-memory")
	}
	if IsSpecified(ctx, "vm-cpukind") {
		guest.CPUKind = GetString(ctx, "vm-cpukind")
	}
	return &guest
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
}
