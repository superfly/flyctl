package launch

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/terminal"
)

type launchPlan struct {
	AppName       string `json:"name" url:"name"`
	appNameSource string

	RegionCode   string `json:"region" url:"region"`
	regionSource string

	OrgSlug   string `json:"org" url:"org"`
	orgSource string

	CPUKind     string `json:"vm_cpukind,omitempty" url:"vm_cpukind,omitempty"`
	CPUs        int    `json:"vm_cpus,omitempty" url:"vm_cpus,omitempty"`
	MemoryMB    int    `json:"vm_memory,omitempty" url:"vm_memory,omitempty"`
	VmSize      string `json:"vm_size,omitempty" url:"vm_size,omitempty"`
	guestSource string

	Postgres       *postgresPlan `json:"-"` // `json:"postgres" url:"postgres"`
	postgresSource string

	Redis       *redisPlan `json:"-"` // `json:"redis" url:"redis"`
	redisSource string

	ScannerFamily string `json:"scanner_family" url:"scanner_family"`

	cache map[string]interface{}
}

func cacheGrab[T any](cache map[string]interface{}, key string, cb func() (T, error)) (T, error) {
	if val, ok := cache[key]; ok {
		return val.(T), nil
	}
	val, err := cb()
	if err != nil {
		return val, err
	}
	cache[key] = val
	return val, nil
}

func (p *launchPlan) Org(ctx context.Context) (*api.Organization, error) {
	apiClient := client.FromContext(ctx).API()
	return cacheGrab(p.cache, "org,"+p.OrgSlug, func() (*api.Organization, error) {
		return apiClient.GetOrganizationBySlug(ctx, p.OrgSlug)
	})
}

func (p *launchPlan) Region(ctx context.Context) (api.Region, error) {

	apiClient := client.FromContext(ctx).API()
	regions, err := cacheGrab(p.cache, "regions", func() ([]api.Region, error) {
		regions, _, err := apiClient.PlatformRegions(ctx)
		if err != nil {
			return nil, err
		}
		return regions, nil
	})
	if err != nil {
		return api.Region{}, err
	}

	region, ok := lo.Find(regions, func(r api.Region) bool {
		return r.Code == p.RegionCode
	})
	if !ok {
		return region, fmt.Errorf("region %s not found", p.RegionCode)
	}
	return region, nil
}

// Summary returns a human-readable summary of the launch plan.
// Used to confirm the plan before executing it.
func (p *launchPlan) Summary(ctx context.Context) (string, error) {

	guest := p.Guest()

	org, err := p.Org(ctx)
	if err != nil {
		return "", err
	}

	region, err := p.Region(ctx)
	if err != nil {
		return "", err
	}

	redisStr, err := p.Redis.Describe(ctx)
	if err != nil {
		return "", err
	}

	rows := [][]string{
		{"Organization", org.Name, p.orgSource},
		{"Name", p.AppName, p.appNameSource},
		{"Region", region.Name, p.regionSource},
		{"App Machines", guest.String(), p.guestSource},
		{"Postgres", p.Postgres.Describe(), p.postgresSource},
		{"Redis", redisStr, p.redisSource},
	}

	colLengths := []int{0, 0, 0}
	for _, row := range rows {
		for i, col := range row {
			if len(col) > colLengths[i] {
				colLengths[i] = len(col)
			}
		}
	}

	ret := ""
	for _, row := range rows {

		label := row[0]
		value := row[1]
		source := row[2]

		labelSpaces := strings.Repeat(" ", colLengths[0]-len(label))
		valueSpaces := strings.Repeat(" ", colLengths[1]-len(value))

		ret += fmt.Sprintf("%s: %s%s %s(%s)\n", label, labelSpaces, value, valueSpaces, source)
	}
	return ret, nil
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
