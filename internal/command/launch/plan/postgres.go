package plan

import (
	fly "github.com/superfly/fly-go"
)

type PostgresPlan struct {
	FlyPostgres      *FlyPostgresPlan      `json:"fly_postgres"`
	SupabasePostgres *SupabasePostgresPlan `json:"supabase_postgres"`
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
	return nil
}

func DefaultPostgres(plan *LaunchPlan) PostgresPlan {
	return PostgresPlan{
		// TODO: Once supabase is GA, we want to default to Supabase
		FlyPostgres: &FlyPostgresPlan{
			// NOTE: Until Legacy Launch is removed, we have to maintain
			//       "%app_name%-db" as the app name for the database.
			//       (legacy launch does not have a single source-of-truth name for the db,
			//        so it constructs the name on-the-spot each time it needs it)
			AppName:    plan.AppName + "-db",
			VmSize:     "shared-cpu-1x",
			VmRam:      256,
			Nodes:      1,
			DiskSizeGB: 1,
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
