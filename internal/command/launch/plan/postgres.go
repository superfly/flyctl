package plan

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
)

type PostgresProvider interface {
	Describe(ctx context.Context) (string, error)
}

type PostgresPlan struct {
	FlyPostgres *FlyPostgresPlan `json:"fly_postgres" url:"fly_postgres"`
}

func (p *PostgresPlan) Provider() PostgresProvider {
	if p == nil {
		return nil
	}
	if p.FlyPostgres != nil {
		return p.FlyPostgres
	}
	return nil
}

func (p *PostgresPlan) Describe(ctx context.Context) (string, error) {
	if provider := p.Provider(); provider != nil {
		return provider.Describe(ctx)
	}
	return descriptionNone, nil
}

func DefaultPostgres(plan *LaunchPlan) PostgresPlan {
	return PostgresPlan{
		FlyPostgres: &FlyPostgresPlan{
			// NOTE: Until Legacy Launch is removed, we have to maintain
			//       "%app_name%-db" as the app name for the database.
			//       (legacy launch does not have a single source-of-truth name for the db,
			//        so it constructs the name on-the-spot each time it needs it)
			AppName:    plan.AppName + "-db",
			VmSize:     "shared-cpu-1x",
			VmRam:      1024,
			Nodes:      1,
			DiskSizeGB: 10,
		},
	}
}

type FlyPostgresPlan struct {
	AppName    string `json:"app_name" url:"app_name"`
	VmSize     string `json:"vm_size" url:"vm_size"`
	VmRam      int    `json:"vm_ram" url:"vm_ram"`
	Nodes      int    `json:"nodes" url:"nodes"`
	DiskSizeGB int    `json:"disk_size_gb" url:"disk_size_gb"`
	AutoStop   bool   `json:"auto_stop" url:"auto_stop"`
}

func (p *FlyPostgresPlan) Guest() *api.MachineGuest {
	guest := api.MachineGuest{}
	guest.SetSize(p.VmSize)
	if p.VmRam != 0 {
		guest.MemoryMB = p.VmRam
	}
	return &guest
}

func (p *FlyPostgresPlan) Describe(ctx context.Context) (string, error) {

	nodePlural := lo.Ternary(p.Nodes == 1, "", "s")
	nodesStr := fmt.Sprintf("%d Node%s", p.Nodes, nodePlural)

	guestStr := p.VmSize
	if p.VmRam > 0 {
		guest := api.MachinePresets[p.VmSize]
		if guest.MemoryMB != p.VmRam {
			guestStr = fmt.Sprintf("%s (%dGB RAM)", guest, p.VmRam/1024)
		}
	}

	diskSizeStr := fmt.Sprintf("%dGB disk", p.DiskSizeGB)

	info := []string{nodesStr, guestStr, diskSizeStr}
	if p.AutoStop {
		info = append(info, "auto-stop")
	}

	return strings.Join(info, ", "), nil
}
