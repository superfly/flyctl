package machine

import (
	"context"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/flag"
)

// ApplyFlagsToGuest applies CLI flags to a Guest
// Returns true if any flags were applied
func ApplyFlagsToGuest(ctx context.Context, guest *api.MachineGuest) bool {
	modified := false
	if flag.IsSpecified(ctx, "vm-size") {
		guest.SetSize(flag.GetString(ctx, "vm-size"))
		modified = true
	}
	if flag.IsSpecified(ctx, "vm-cpus") {
		guest.CPUs = flag.GetInt(ctx, "vm-cpus")
		modified = true
	}
	if flag.IsSpecified(ctx, "vm-memory") {
		guest.MemoryMB = flag.GetInt(ctx, "vm-memory")
		modified = true
	}
	if flag.IsSpecified(ctx, "vm-cpukind") {
		guest.CPUKind = flag.GetString(ctx, "vm-cpukind")
		modified = true
	}
	return modified
}

var VMSizeFlags = flag.Set{
	flag.String{
		Name:        "vm-size",
		Description: `The VM size to set machines to. See "fly platform vm-sizes" for valid values`,
		Aliases:     []string{"size"},
	},
	flag.Int{
		Name:        "vm-cpus",
		Description: "Number of CPUs",
		Aliases:     []string{"cpus"},
	},
	flag.String{
		Name:        "vm-cpukind",
		Description: "The kind of CPU to use ('shared' or 'performance')",
	},
	flag.Int{
		Name:        "vm-memory",
		Description: "Memory (in megabytes) to attribute to the VM",
		Aliases:     []string{"memory"},
	},
}
