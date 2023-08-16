package plan

import (
	"github.com/superfly/flyctl/api"
)

const descriptionNone = "<none>"

type LaunchPlan struct {
	AppName string `json:"name" url:"name"`

	RegionCode string `json:"region" url:"region"`

	OrgSlug string `json:"org" url:"org"`

	CPUKind  string `json:"vm_cpukind,omitempty" url:"vm_cpukind,omitempty"`
	CPUs     int    `json:"vm_cpus,omitempty" url:"vm_cpus,omitempty"`
	MemoryMB int    `json:"vm_memory,omitempty" url:"vm_memory,omitempty"`
	VmSize   string `json:"vm_size,omitempty" url:"vm_size,omitempty"`

	Postgres postgresPlan `json:"postgres" url:"postgres"`

	Redis redisPlan `json:"redis" url:"redis"`

	ScannerFamily string `json:"scanner_family" url:"scanner_family"`
}

func (p *LaunchPlan) Guest() *api.MachineGuest {
	// TODO(Allison): Determine whether we should use VmSize or CPUKind/CPUs
	guest := api.MachineGuest{
		CPUs:    p.CPUs,
		CPUKind: p.CPUKind,
	}
	if false {
		guest.SetSize(p.VmSize)
	}
	guest.MemoryMB = p.MemoryMB
	return &guest
}

func (p *LaunchPlan) SetGuestFields(guest *api.MachineGuest) {
	p.CPUs = guest.CPUs
	p.CPUKind = guest.CPUKind
	p.MemoryMB = guest.MemoryMB
	p.VmSize = guest.ToSize()
}
