package launch

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
)

type postgresPlan struct {
	FlyPostgres *flyPostgresPlan `json:"fly_postgres" url:"fly_postgres"`
}

func (p *postgresPlan) Describe(ctx context.Context) (string, error) {
	if p == nil {
		return descriptionNone, nil
	}
	if p.FlyPostgres != nil {
		return p.FlyPostgres.Describe()
	}
	return descriptionNone, nil
}

type flyPostgresPlan struct {
	VmSize     string `json:"vm_size" url:"vm_size"`
	Nodes      int    `json:"nodes" url:"nodes"`
	DiskSizeGB int    `json:"disk_size_gb" url:"disk_size_gb"`
}

func (p *flyPostgresPlan) Guest() *api.MachineGuest {
	guest := api.MachineGuest{}
	guest.SetSize(p.VmSize)
	return &guest
}

func (p *flyPostgresPlan) Describe() (string, error) {
	nodePlural := lo.Ternary(p.Nodes == 1, "", "s")
	return fmt.Sprintf("%d Node%s, %s, %dGB disk", p.Nodes, nodePlural, p.Guest().String(), p.DiskSizeGB), nil
}
