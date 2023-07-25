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
	AppName       string `json:"app_name"`
	appNameSource string

	Region       *api.Region `json:"region"`
	regionSource string

	Org       *api.Organization `json:"org"`
	orgSource string

	Guest       *api.MachineGuest `json:"guest"`
	guestSource string

	Postgres       *postgresPlan `json:"postgres"`
	postgresSource string

	Redis       *redisPlan `json:"redis"`
	redisSource string

	ScannerFamily string `json:"scanner_family"`
}

// Summary returns a human-readable summary of the launch plan.
// Used to confirm the plan before executing it.
func (p *launchPlan) Summary(ctx context.Context) string {

	guest := p.Guest
	if guest == nil {
		guest = api.MachinePresets["shared-cpu-1x"]
	}

	rows := [][]string{
		{"Organization", p.Org.Name, p.orgSource},
		{"Name", p.AppName, p.appNameSource},
		{"Region", p.Region.Name, p.regionSource},
		{"App Machines", guest.String(), p.guestSource},
		{"Postgres", p.Postgres.String(), p.postgresSource},
		{"Redis", p.Redis.String(ctx), p.redisSource},
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
	return ret
}

// TODO
type postgresPlan struct {
	Guest      *api.MachineGuest `json:"guest"`
	Nodes      int               `json:"nodes"`
	DiskSizeGB int               `json:"disk_size_gb"`
}

func (p *postgresPlan) String() string {
	if p == nil {
		return "<none>"
	}
	nodePlural := lo.Ternary(p.Nodes == 1, "", "s")
	return fmt.Sprintf("%d Node%s, %s, %dGB disk", p.Nodes, nodePlural, p.Guest.String(), p.DiskSizeGB)
}

type redisPlan struct {
	PlanId       string       `json:"plan_id"`
	Eviction     bool         `json:"eviction"`
	ReadReplicas []api.Region `json:"read_replicas"`
}

func (p *redisPlan) String(ctx context.Context) string {
	if p == nil {
		return "<none>"
	}

	client := client.FromContext(ctx)

	result, err := gql.ListAddOnPlans(ctx, client.API().GenqClient)
	if err != nil {
		terminal.Debugf("Failed to list addon plans: %s\n", err)
		return "<internal error>"
	}

	for _, plan := range result.AddOnPlans.Nodes {
		if plan.Id == p.PlanId {
			evictionStatus := lo.Ternary(p.Eviction, "enabled", "disabled")
			return fmt.Sprintf("%s: %s Max Data Size, ($%d / month), eviction %s", plan.DisplayName, plan.MaxDataSize, plan.PricePerMonth, evictionStatus)
		}
	}

	return "<plan not found, this is an error>"
}
