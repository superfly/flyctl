package plan

import (
	"context"
	"fmt"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command/mpg"
	"github.com/superfly/flyctl/iostreams"
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

// DefaultPostgres returns the default postgres configuration, preferring managed postgres when available in the region
func DefaultPostgres(ctx context.Context, plan *LaunchPlan, mpgEnabled bool) PostgresPlan {
	// If managed postgres is enabled, check if it's available in the region
	if mpgEnabled {
		validRegion, err := mpg.IsValidMPGRegion(ctx, plan.OrgSlug, plan.RegionCode)
		if err == nil && validRegion {
			// Managed postgres is available in this region, use it
			return PostgresPlan{
				ManagedPostgres: &ManagedPostgresPlan{
					DbName:   plan.AppName + "-db",
					Region:   plan.RegionCode,
					Plan:     "basic",
					DiskSize: diskSizeGb,
				},
			}
		}

		// Managed postgres is not available in this region, log warning and fall back to FlyPostgres
		io := iostreams.FromContext(ctx)
		if io != nil {
			fmt.Fprintf(io.ErrOut, "Warning: Using Unmanaged Postgres because Managed Postgres isn't yet available in region %s\n", plan.RegionCode)
		}
	}

	vmRam := 256
	diskSizeGb := 1
	price := -1

	// Use FlyPostgres (either because mpgEnabled is false or managed postgres is not available in region)
	return PostgresPlan{
		// TODO: Once supabase is GA, we want to default to Supabase
		FlyPostgres: &FlyPostgresPlan{
			// NOTE: Until Legacy Launch is removed, we have to maintain
			//       "%app_name%-db" as the app name for the database.
			//       (legacy launch does not have a single source-of-truth name for the db,
			//        so it constructs the name on-the-spot each time it needs it)
			AppName:    plan.AppName + "-db",
			VmSize:     "shared-cpu-1x",
			VmRam:      vmRam,
			Nodes:      1,
			DiskSizeGB: diskSizeGb,
			Price:      price,
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
	Price      int    `json:"price"`
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
	DbName    string `json:"db_name"`
	Region    string `json:"region"`
	Plan      string `json:"plan"`
	DiskSize  int    `json:"disk_size"`
	ClusterID string `json:"cluster_id,omitempty"`
}

func (p *ManagedPostgresPlan) GetDbName(plan *LaunchPlan) string {
	if p.DbName == "" {
		return plan.AppName + "-db"
	}
	return p.DbName
}

func (p *ManagedPostgresPlan) GetRegion(plan *LaunchPlan) string {
	if p.Region == "" {
		return plan.RegionCode
	}
	return p.Region
}
