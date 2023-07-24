package launch

import (
	"fmt"
	"strings"

	"github.com/superfly/flyctl/api"
)

type launchPlan struct {
	AppName       string
	appNameSource string

	Region       *api.Region
	regionSource string

	Org       *api.Organization
	orgSource string

	Guest       *api.MachineGuest
	guestSource string

	Postgres       *postgresPlan
	postgresSource string

	Redis       *redisPlan
	redisSource string

	ScannerFamily string
}

// Summary returns a human-readable summary of the launch plan.
// Used to confirm the plan before executing it.
func (p *launchPlan) Summary() string {

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
		{"Redis", p.Redis.String(), p.redisSource},
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
type postgresPlan struct{}

func (p *postgresPlan) String() string {
	if p == nil {
		return "<none>"
	}
	return "unimplemented"
}

type redisPlan struct{}

func (p *redisPlan) String() string {
	if p == nil {
		return "<none>"
	}
	return "unimplemented"
}
