package plan

import (
	fly "github.com/superfly/fly-go"
)

type PostgresPlan struct {
	FlyPostgres      *FlyPostgresPlan      `json:"fly_postgres"`
	SupabasePostgres *SupabasePostgresPlan `json:"supabase_postgres"`
	ManagedPostgres  *ManagedPostgresPlan  `json:"managed_postgres"`
}

func (p *PostgresPlan) Provider() any {
	if p == nil {
		return nil
	}
	if p.FlyPostgres != nil {
		return p.FlyPostgres
	}
	if p.SupabasePostgres != nil {
		return p.SupabasePostgres
	}
	if p.ManagedPostgres != nil {
		return p.ManagedPostgres
	}
	return nil
}

func DefaultPostgres(plan *LaunchPlan) PostgresPlan {
	return PostgresPlan{
		ManagedPostgres: &ManagedPostgresPlan{
			DbName: plan.AppName + "-db",
			Region: "iad",
			Plan:   "basic",
			Disk:   10,
		},
	}
}

type FlyPostgresPlan struct {
	AppName    string `json:"app_name"`
	VmSize     string `json:"vm_size"`
	VmRam      int    `json:"vm_ram"`
	Nodes      int    `json:"nodes"`
	DiskSizeGB int    `json:"disk_size_gb"`
	AutoStop   bool   `json:"auto_stop"`
}

func (p *FlyPostgresPlan) Guest() *fly.MachineGuest {
	guest := fly.MachineGuest{}
	guest.SetSize(p.VmSize)
	if p.VmRam != 0 {
		guest.MemoryMB = p.VmRam
	}
	return &guest
}

type SupabasePostgresPlan struct {
	DbName string `json:"db_name"`
	Region string `json:"region"`
}

func (p *SupabasePostgresPlan) GetDbName(plan *LaunchPlan) string {
	if p.DbName == "" {
		return plan.AppName + "-db"
	}
	return p.DbName
}

func (p *SupabasePostgresPlan) GetRegion(plan *LaunchPlan) string {
	if p.Region == "" {
		return plan.RegionCode
	}
	return p.Region
}

type ManagedPostgresPlan struct {
	ExistingMpgHashid string `json:"existing_mpg_hashid"`
	DbName            string `json:"db_name"`
	Region            string `json:"region"`
	Plan              string `json:"plan"`
	Disk              int    `json:"disk"`
}

func (p *ManagedPostgresPlan) GetRegion() string {
	if p.Region == "" {
		return "iad"
	}
	return p.Region
}

func (p *ManagedPostgresPlan) GetPlan() string {
	if p.Plan == "" {
		return "basic"
	}
	return p.Plan
}

func (p *ManagedPostgresPlan) GetDisk() int {
	if p.Disk == 0 {
		return 10
	}
	return p.Disk
}
