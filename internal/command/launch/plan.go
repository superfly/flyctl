package launch

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/terminal"
)

type launchPlanSource struct {
	appNameSource  string
	regionSource   string
	orgSource      string
	guestSource    string
	postgresSource string
	redisSource    string
}

type launchPlan struct {
	AppName string `json:"name" url:"name"`

	RegionCode string `json:"region" url:"region"`

	OrgSlug string `json:"org" url:"org"`

	CPUKind  string `json:"vm_cpukind,omitempty" url:"vm_cpukind,omitempty"`
	CPUs     int    `json:"vm_cpus,omitempty" url:"vm_cpus,omitempty"`
	MemoryMB int    `json:"vm_memory,omitempty" url:"vm_memory,omitempty"`
	VmSize   string `json:"vm_size,omitempty" url:"vm_size,omitempty"`

	Postgres *postgresPlan `json:"-"` // `json:"postgres" url:"postgres"`

	Redis *redisPlan `json:"-"` // `json:"redis" url:"redis"`

	ScannerFamily string `json:"scanner_family" url:"scanner_family"`
}

func (p *launchPlan) Guest() *api.MachineGuest {
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

func (p *launchPlan) SetGuestFields(guest *api.MachineGuest) {
	p.CPUs = guest.CPUs
	p.CPUKind = guest.CPUKind
	p.MemoryMB = guest.MemoryMB
	p.VmSize = guest.ToSize()
}

// TODO
type postgresPlan struct {
	VmSize     string `json:"vm_size" url:"vm_size"`
	Nodes      int    `json:"nodes" url:"nodes"`
	DiskSizeGB int    `json:"disk_size_gb" url:"disk_size_gb"`
}

func (p *postgresPlan) Guest() *api.MachineGuest {
	guest := api.MachineGuest{}
	guest.SetSize(p.VmSize)
	return &guest
}

func (p *postgresPlan) Describe() string {
	if p == nil {
		return "<none>"
	}
	nodePlural := lo.Ternary(p.Nodes == 1, "", "s")
	return fmt.Sprintf("%d Node%s, %s, %dGB disk", p.Nodes, nodePlural, p.Guest().String(), p.DiskSizeGB)
}

type redisPlan struct {
	PlanId       string   `json:"plan_id" url:"plan_id"`
	Eviction     bool     `json:"eviction" url:"eviction"`
	ReadReplicas []string `json:"read_replicas" url:"read_replicas"`
}

func (p *redisPlan) Describe(ctx context.Context) (string, error) {
	if p == nil {
		return "<none>", nil
	}

	client := client.FromContext(ctx)

	result, err := gql.ListAddOnPlans(ctx, client.API().GenqClient)
	if err != nil {
		terminal.Debugf("Failed to list addon plans: %s\n", err)
		return "<internal error>", err
	}

	for _, plan := range result.AddOnPlans.Nodes {
		if plan.Id == p.PlanId {
			evictionStatus := lo.Ternary(p.Eviction, "enabled", "disabled")
			return fmt.Sprintf("%s: %s Max Data Size, ($%d / month), eviction %s", plan.DisplayName, plan.MaxDataSize, plan.PricePerMonth, evictionStatus), nil
		}
	}

	return "<plan not found, this is an error>", fmt.Errorf("plan not found: %s", p.PlanId)
}
